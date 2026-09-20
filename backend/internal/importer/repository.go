package importer

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
)

type Repository struct{ pool db.Querier }

var ErrExtractionNotFound = errors.New("importer: Extraction 不存在")

func NewRepository(pool db.Querier) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(tx pgx.Tx) db.Querier {
	if tx != nil {
		return tx
	}
	return r.pool
}

const jobColumns = `id,job_type,status,initiated_by,idempotency_key,config_json,
	planning_input_json,planning_idempotency_key,source_version_id,proposal_id,
	action_required,current_plan_id,confirmed_plan_id,plan_confirmed_at,
	current_stage,progress,error_json,created_at,started_at,finished_at,updated_at`

func scanJob(row pgx.Row) (*Job, error) {
	var job Job
	err := row.Scan(&job.ID, &job.JobType, &job.Status, &job.InitiatedBy,
		&job.IdempotencyKey, &job.Config, &job.PlanningInput,
		&job.PlanningIdempotencyKey, &job.SourceVersionID, &job.ProposalID,
		&job.ActionRequired, &job.CurrentPlanID, &job.ConfirmedPlanID,
		&job.PlanConfirmedAt, &job.CurrentStage, &job.Progress, &job.Error,
		&job.CreatedAt, &job.StartedAt, &job.FinishedAt, &job.UpdatedAt)
	return &job, err
}

func (r *Repository) GetJob(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Job, error) {
	job, err := scanJob(r.q(tx).QueryRow(ctx, `SELECT `+jobColumns+` FROM import_job WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	return job, err
}

func (r *Repository) GetJobForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Job, error) {
	job, err := scanJob(r.q(tx).QueryRow(ctx, `SELECT `+jobColumns+` FROM import_job WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	return job, err
}

func (r *Repository) NextQueuedJobForUpdate(ctx context.Context, tx pgx.Tx) (*Job, error) {
	job, err := scanJob(r.q(tx).QueryRow(ctx, `SELECT `+jobColumns+` FROM import_job
		WHERE status='queued' AND job_type='source_import'
		ORDER BY created_at,id FOR UPDATE SKIP LOCKED LIMIT 1`))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoQueuedJob
	}
	return job, err
}

func (r *Repository) NextStaleRunningJobForUpdate(
	ctx context.Context, tx pgx.Tx, staleBefore time.Time,
) (*Job, error) {
	job, err := scanJob(r.q(tx).QueryRow(ctx, `SELECT `+jobColumns+` FROM import_job
		WHERE status='running' AND job_type='source_import' AND updated_at < $1
		ORDER BY updated_at,id FOR UPDATE SKIP LOCKED LIMIT 1`, staleBefore))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoQueuedJob
	}
	return job, err
}

func (r *Repository) GetJobByKey(ctx context.Context, actorID uuid.UUID, key string) (*Job, error) {
	job, err := scanJob(r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM import_job
		WHERE initiated_by=$1 AND idempotency_key=$2`, actorID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	return job, err
}

func (r *Repository) HasProposal(ctx context.Context, tx pgx.Tx, jobID uuid.UUID) (bool, error) {
	var exists bool
	err := r.q(tx).QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM proposal WHERE import_job_id=$1)`,
		jobID,
	).Scan(&exists)
	return exists, err
}

type jobListCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

func encodeJobCursor(value jobListCursor) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeJobCursor(cursor string) (jobListCursor, error) {
	var value jobListCursor
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	return value, nil
}

func (r *Repository) ListOwnedJobs(
	ctx context.Context, actorID uuid.UUID, status, cursor string, limit int,
) (*JobPage, error) {
	var afterTime *time.Time
	afterID := uuid.Nil
	if cursor != "" {
		after, err := decodeJobCursor(cursor)
		if err != nil || after.CreatedAt.IsZero() || after.ID == uuid.Nil {
			return nil, ErrInvalidCursor
		}
		afterTime = &after.CreatedAt
		afterID = after.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+jobColumns+` FROM import_job
		WHERE initiated_by=$1
		  AND ($2='' OR status=$2)
		  AND ($3::timestamptz IS NULL OR (created_at,id) < ($3,$4))
		ORDER BY created_at DESC,id DESC
		LIMIT $5`, actorID, status, afterTime, afterID, limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &JobPage{Items: make([]Job, 0, limit)}
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		result.Items = append(result.Items, *job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeJobCursor(jobListCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (r *Repository) InsertJobIfAbsent(ctx context.Context, job *Job) (bool, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO import_job
		(id,job_type,status,initiated_by,idempotency_key,config_json,planning_input_json,current_stage,progress)
		VALUES ($1,$2,'queued',$3,$4,$5::jsonb,$6::jsonb,'queued',0)
		ON CONFLICT (initiated_by,idempotency_key) DO NOTHING
		RETURNING created_at,updated_at`, job.ID, job.JobType, job.InitiatedBy,
		job.IdempotencyKey, job.Config, job.PlanningInput).Scan(&job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

type ParseCheckpoint struct {
	SourceVersionID uuid.UUID
	ContentHash     string
}

// FindLatestParseCheckpoint returns the newest immutable parse artifact
// produced by this job. Successful and intentionally skipped parse stages are
// both valid checkpoints; nil means the job has never completed parsing.
func (r *Repository) FindLatestParseCheckpoint(ctx context.Context, jobID uuid.UUID) (*ParseCheckpoint, error) {
	var rawVersionID string
	var contentHash string
	err := r.pool.QueryRow(ctx, `SELECT s.output_hash,s.input_hash FROM import_stage_run s
		JOIN import_run r ON r.id=s.import_run_id
		WHERE r.import_job_id=$1 AND s.stage=$2
		  AND s.status IN ($3,$4) AND s.output_hash IS NOT NULL AND s.input_hash IS NOT NULL
		ORDER BY r.attempt DESC,s.finished_at DESC,s.id DESC LIMIT 1`,
		jobID, StageParse, StageSucceeded, StageSkipped).Scan(&rawVersionID, &contentHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	parsed, err := uuid.Parse(rawVersionID)
	if err != nil || parsed == uuid.Nil || !validSHA256(contentHash) {
		return nil, fmt.Errorf("%w: parse checkpoint", ErrInvalidJob)
	}
	return &ParseCheckpoint{SourceVersionID: parsed, ContentHash: contentHash}, nil
}

func scanRun(row pgx.Row) (*Run, error) {
	var run Run
	err := row.Scan(&run.ID, &run.ImportJobID, &run.Attempt, &run.IdempotencyKey,
		&run.Status, &run.Error, &run.StartedAt, &run.FinishedAt)
	return &run, err
}

func (r *Repository) GetRunByKey(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, key string) (*Run, error) {
	run, err := scanRun(r.q(tx).QueryRow(ctx, `SELECT id,import_job_id,attempt,idempotency_key,
		status,error_json,started_at,finished_at FROM import_run
		WHERE import_job_id=$1 AND idempotency_key=$2`, jobID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	return run, err
}

func (r *Repository) NextAttempt(ctx context.Context, tx pgx.Tx, jobID uuid.UUID) (int, error) {
	var attempt int
	err := r.q(tx).QueryRow(ctx, `SELECT COALESCE(max(attempt),0)+1 FROM import_run WHERE import_job_id=$1`, jobID).Scan(&attempt)
	return attempt, err
}

func (r *Repository) InsertRun(ctx context.Context, tx pgx.Tx, run *Run) error {
	return r.q(tx).QueryRow(ctx, `INSERT INTO import_run
		(id,import_job_id,attempt,idempotency_key,status) VALUES ($1,$2,$3,$4,'running')
		RETURNING started_at`, run.ID, run.ImportJobID, run.Attempt, run.IdempotencyKey).Scan(&run.StartedAt)
}

func (r *Repository) StartJob(ctx context.Context, tx pgx.Tx, jobID uuid.UUID) error {
	command, err := r.q(tx).Exec(ctx, `UPDATE import_job SET status='running',started_at=COALESCE(started_at,now()),
		finished_at=NULL,error_json=NULL,action_required=NULL,updated_at=now()
		WHERE id=$1 AND status IN ('queued','failed','cancelled')`, jobID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) InsertStage(ctx context.Context, tx pgx.Tx, stage *StageRun) error {
	return r.q(tx).QueryRow(ctx, `INSERT INTO import_stage_run
		(id,import_run_id,stage,status,input_hash) VALUES ($1,$2,$3,'running',$4)
		RETURNING started_at`, stage.ID, stage.ImportRunID, stage.Stage, stage.InputHash).Scan(&stage.StartedAt)
}

func (r *Repository) CompleteStage(ctx context.Context, tx pgx.Tx, stageID uuid.UUID, status string, outputHash *string, errorJSON []byte) error {
	command, err := r.q(tx).Exec(ctx, `UPDATE import_stage_run SET status=$2,output_hash=$3,error_json=$4::jsonb,
		finished_at=now() WHERE id=$1 AND status='running'`, stageID, status, outputHash, errorJSON)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) AdvanceJob(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, stage string, progress int) error {
	_, err := r.q(tx).Exec(ctx, `UPDATE import_job SET current_stage=$2,progress=$3,updated_at=now() WHERE id=$1`, jobID, stage, progress)
	return err
}

func (r *Repository) AdvanceParsedJob(
	ctx context.Context,
	tx pgx.Tx,
	jobID, sourceVersionID uuid.UUID,
	progress int,
) error {
	command, err := r.q(tx).Exec(ctx, `UPDATE import_job
		SET source_version_id=$2,current_stage=$3,progress=$4,updated_at=now()
		WHERE id=$1 AND status='running'`,
		jobID, sourceVersionID, StageParse, progress)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) FinishRun(ctx context.Context, tx pgx.Tx, runID uuid.UUID, status string, errorJSON []byte) error {
	_, err := r.q(tx).Exec(ctx, `UPDATE import_run SET status=$2,error_json=$3::jsonb,finished_at=now()
		WHERE id=$1 AND status='running'`, runID, status, errorJSON)
	return err
}

func (r *Repository) CancelRunningStages(ctx context.Context, tx pgx.Tx, runID uuid.UUID, errorJSON []byte) error {
	_, err := r.q(tx).Exec(ctx, `UPDATE import_stage_run SET status=$2,error_json=$3::jsonb,finished_at=now()
		WHERE import_run_id=$1 AND status='running'`, runID, StageCancelled, errorJSON)
	return err
}

func (r *Repository) FinishJob(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, status, stage string,
	progress int, sourceVersionID, proposalID *uuid.UUID, errorJSON []byte) error {
	_, err := r.q(tx).Exec(ctx, `UPDATE import_job SET status=$2,current_stage=$3,progress=$4,
		source_version_id=COALESCE($5,source_version_id),proposal_id=COALESCE($6,proposal_id),
		action_required=NULL,error_json=$7::jsonb,finished_at=now(),updated_at=now() WHERE id=$1`,
		jobID, status, stage, progress, sourceVersionID, proposalID, errorJSON)
	return err
}

func (r *Repository) RequireAction(
	ctx context.Context,
	tx pgx.Tx,
	jobID, sourceVersionID uuid.UUID,
	action string,
) error {
	command, err := r.q(tx).Exec(ctx, `UPDATE import_job
		SET status=$2,source_version_id=$3,action_required=$4,current_stage=$5,
			progress=$6,error_json=NULL,finished_at=NULL,updated_at=now()
		WHERE id=$1 AND status='running'`,
		jobID, JobActionRequired, sourceVersionID, action, StagePlan,
		stageProgress[StagePlan])
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) ConfirmPlan(
	ctx context.Context,
	tx pgx.Tx,
	jobID, planID uuid.UUID,
) error {
	command, err := r.q(tx).Exec(ctx, `UPDATE import_job
		SET status='queued',confirmed_plan_id=$2,plan_confirmed_at=now(),
			action_required=NULL,current_stage='queued',progress=0,error_json=NULL,
			finished_at=NULL,updated_at=now()
		WHERE id=$1 AND status='action_required' AND action_required='confirm_plan'
			AND current_plan_id=$2`,
		jobID, planID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) CompletePlanStage(
	ctx context.Context,
	tx pgx.Tx,
	jobID uuid.UUID,
	stageID uuid.UUID,
	planID uuid.UUID,
	autoConfirm bool,
) error {
	output := planID.String()
	if err := r.CompleteStage(ctx, tx, stageID, StageSucceeded, &output, nil); err != nil {
		return err
	}
	command, err := r.q(tx).Exec(ctx, `UPDATE import_job
		SET current_plan_id=$2,
			confirmed_plan_id=CASE WHEN $3 THEN $2 ELSE confirmed_plan_id END,
			plan_confirmed_at=CASE WHEN $3 THEN now() ELSE plan_confirmed_at END,
			current_stage=$4,progress=$5,updated_at=now()
		WHERE id=$1 AND status='running'`,
		jobID, planID, autoConfirm, StagePlan, stageProgress[StagePlan])
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) RequeueForPlanning(
	ctx context.Context,
	tx pgx.Tx,
	jobID uuid.UUID,
	input json.RawMessage,
	idempotencyKey string,
) error {
	command, err := r.q(tx).Exec(ctx, `UPDATE import_job
		SET status='queued',planning_input_json=$2::jsonb,
			planning_idempotency_key=$3,action_required=NULL,
			confirmed_plan_id=NULL,plan_confirmed_at=NULL,
			current_stage='queued',progress=0,error_json=NULL,
			finished_at=NULL,updated_at=now()
		WHERE id=$1 AND proposal_id IS NULL
			AND status IN ('action_required','failed','cancelled','succeeded')`,
		jobID, input, idempotencyKey)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) RunningRun(ctx context.Context, tx pgx.Tx, jobID uuid.UUID) (*Run, error) {
	run, err := scanRun(r.q(tx).QueryRow(ctx, `SELECT id,import_job_id,attempt,idempotency_key,
		status,error_json,started_at,finished_at FROM import_run
		WHERE import_job_id=$1 AND status='running' ORDER BY attempt DESC LIMIT 1`, jobID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrRunNotFound
	}
	return run, err
}

func (r *Repository) RequeueJob(ctx context.Context, tx pgx.Tx, jobID uuid.UUID) error {
	command, err := r.q(tx).Exec(ctx, `UPDATE import_job SET status='queued',current_stage='queued',progress=0,
		error_json=NULL,finished_at=NULL,updated_at=now() WHERE id=$1 AND status IN ('failed','cancelled')`, jobID)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) ListRuns(ctx context.Context, jobID uuid.UUID) ([]Run, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,import_job_id,attempt,idempotency_key,status,
		error_json,started_at,finished_at FROM import_run WHERE import_job_id=$1 ORDER BY attempt`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []Run{}
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *run)
	}
	return result, rows.Err()
}

func (r *Repository) ListStages(ctx context.Context, jobID uuid.UUID) ([]StageRun, error) {
	rows, err := r.pool.Query(ctx, `SELECT s.id,s.import_run_id,s.stage,s.status,s.input_hash,
		s.output_hash,s.error_json,s.started_at,s.finished_at FROM import_stage_run s
		JOIN import_run r ON r.id=s.import_run_id WHERE r.import_job_id=$1
		ORDER BY r.attempt,s.started_at`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []StageRun{}
	for rows.Next() {
		var stage StageRun
		if err := rows.Scan(&stage.ID, &stage.ImportRunID, &stage.Stage, &stage.Status,
			&stage.InputHash, &stage.OutputHash, &stage.Error, &stage.StartedAt, &stage.FinishedAt); err != nil {
			return nil, err
		}
		result = append(result, stage)
	}
	return result, rows.Err()
}

func (r *Repository) Actor(ctx context.Context, id uuid.UUID) (string, string, error) {
	var actorType, status string
	err := r.pool.QueryRow(ctx, `SELECT actor_type,status FROM actor WHERE id=$1`, id).Scan(&actorType, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", fmt.Errorf("%w: actor", ErrInvalidJob)
	}
	return actorType, status, err
}

func (r *Repository) GetExtraction(ctx context.Context, sourceVersionID uuid.UUID) (*Extraction, error) {
	var extraction Extraction
	err := r.pool.QueryRow(ctx, `SELECT id,source_version_id,schema_version,prompt_key,
		prompt_version,model,candidates_json,quality_score,created_at FROM import_extraction
		WHERE source_version_id=$1 AND schema_version=1`, sourceVersionID).Scan(
		&extraction.ID, &extraction.SourceVersionID, &extraction.SchemaVersion, &extraction.PromptKey,
		&extraction.PromptVersion, &extraction.Model, &extraction.CandidatesJSON,
		&extraction.QualityScore, &extraction.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrExtractionNotFound
	}
	return &extraction, err
}

func (r *Repository) InsertExtractionIfAbsent(ctx context.Context, extraction *Extraction) (bool, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO import_extraction
		(id,source_version_id,schema_version,prompt_key,prompt_version,model,candidates_json,quality_score)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8)
		ON CONFLICT (source_version_id,schema_version) DO NOTHING RETURNING created_at`,
		extraction.ID, extraction.SourceVersionID, extraction.SchemaVersion, extraction.PromptKey,
		extraction.PromptVersion, extraction.Model, extraction.CandidatesJSON,
		extraction.QualityScore).Scan(&extraction.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) GetImportPlan(
	ctx context.Context,
	importJobID uuid.UUID,
	inputHash string,
) (*ImportPlanRecord, error) {
	return scanImportPlan(r.pool.QueryRow(ctx, `SELECT id,import_job_id,source_version_id,
		revision,parent_plan_id,planning_input_json,input_hash,schema_version,
		prompt_key,prompt_version,model,plan_json,quality_score,created_by,created_at
		FROM import_plan WHERE import_job_id=$1 AND input_hash=$2 AND schema_version=1`,
		importJobID, inputHash))
}

func (r *Repository) GetImportPlanByID(
	ctx context.Context,
	importJobID, planID uuid.UUID,
) (*ImportPlanRecord, error) {
	return scanImportPlan(r.pool.QueryRow(ctx, `SELECT id,import_job_id,source_version_id,
		revision,parent_plan_id,planning_input_json,input_hash,schema_version,
		prompt_key,prompt_version,model,plan_json,quality_score,created_by,created_at
		FROM import_plan WHERE import_job_id=$1 AND id=$2`,
		importJobID, planID))
}

func scanImportPlan(row pgx.Row) (*ImportPlanRecord, error) {
	var plan ImportPlanRecord
	err := row.Scan(
		&plan.ID, &plan.ImportJobID, &plan.SourceVersionID, &plan.Revision,
		&plan.ParentPlanID, &plan.PlanningInput, &plan.InputHash,
		&plan.SchemaVersion, &plan.PromptKey, &plan.PromptVersion, &plan.Model,
		&plan.PlanJSON, &plan.QualityScore, &plan.CreatedBy, &plan.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrImportPlanNotFound
	}
	return &plan, err
}

func (r *Repository) GetLatestImportPlan(ctx context.Context, importJobID uuid.UUID) (*ImportPlanRecord, error) {
	return scanImportPlan(r.pool.QueryRow(ctx, `SELECT id,import_job_id,source_version_id,
		revision,parent_plan_id,planning_input_json,input_hash,schema_version,
		prompt_key,prompt_version,model,plan_json,quality_score,created_by,created_at
		FROM import_plan WHERE import_job_id=$1 AND schema_version=1
		ORDER BY revision DESC LIMIT 1`, importJobID))
}

func (r *Repository) ListImportPlans(
	ctx context.Context,
	importJobID uuid.UUID,
) ([]ImportPlanRecord, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,import_job_id,source_version_id,
		revision,parent_plan_id,planning_input_json,input_hash,schema_version,
		prompt_key,prompt_version,model,plan_json,quality_score,created_by,created_at
		FROM import_plan WHERE import_job_id=$1 AND schema_version=1
		ORDER BY revision`, importJobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ImportPlanRecord{}
	for rows.Next() {
		plan, err := scanImportPlan(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *plan)
	}
	return result, rows.Err()
}

func (r *Repository) InsertImportPlanIfAbsent(ctx context.Context, plan *ImportPlanRecord) (bool, error) {
	if plan == nil {
		return false, ErrInvalidJob
	}
	err := r.pool.QueryRow(ctx, `INSERT INTO import_plan
		(id,import_job_id,source_version_id,revision,parent_plan_id,
		 planning_input_json,input_hash,schema_version,prompt_key,prompt_version,
		 model,plan_json,quality_score,created_by)
		VALUES ($1,$2,$3,
			(SELECT COALESCE(max(revision),0)+1 FROM import_plan WHERE import_job_id=$2),
			(SELECT id FROM import_plan WHERE import_job_id=$2 ORDER BY revision DESC LIMIT 1),
			$4::jsonb,$5,$6,$7,$8,$9,$10::jsonb,$11,$12)
		ON CONFLICT (import_job_id,input_hash,schema_version) DO NOTHING
		RETURNING revision,parent_plan_id,created_at`,
		plan.ID, plan.ImportJobID, plan.SourceVersionID,
		plan.PlanningInput, plan.InputHash, plan.SchemaVersion, plan.PromptKey,
		plan.PromptVersion, plan.Model, plan.PlanJSON, plan.QualityScore,
		plan.CreatedBy,
	).Scan(&plan.Revision, &plan.ParentPlanID, &plan.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
