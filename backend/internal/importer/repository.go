package importer

import (
	"context"
	"errors"
	"fmt"

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
	source_version_id,proposal_id,current_stage,progress,error_json,created_at,
	started_at,finished_at,updated_at`

func scanJob(row pgx.Row) (*Job, error) {
	var job Job
	err := row.Scan(&job.ID, &job.JobType, &job.Status, &job.InitiatedBy,
		&job.IdempotencyKey, &job.Config, &job.SourceVersionID, &job.ProposalID,
		&job.CurrentStage, &job.Progress, &job.Error, &job.CreatedAt, &job.StartedAt,
		&job.FinishedAt, &job.UpdatedAt)
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

func (r *Repository) GetJobByKey(ctx context.Context, actorID uuid.UUID, key string) (*Job, error) {
	job, err := scanJob(r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM import_job
		WHERE initiated_by=$1 AND idempotency_key=$2`, actorID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	return job, err
}

func (r *Repository) InsertJobIfAbsent(ctx context.Context, job *Job) (bool, error) {
	err := r.pool.QueryRow(ctx, `INSERT INTO import_job
		(id,job_type,status,initiated_by,idempotency_key,config_json,current_stage,progress)
		VALUES ($1,$2,'queued',$3,$4,$5::jsonb,'queued',0)
		ON CONFLICT (initiated_by,idempotency_key) DO NOTHING
		RETURNING created_at,updated_at`, job.ID, job.JobType, job.InitiatedBy,
		job.IdempotencyKey, job.Config).Scan(&job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) FindSucceededByVersion(ctx context.Context, jobType string, sourceVersionID uuid.UUID) (*Job, error) {
	job, err := scanJob(r.pool.QueryRow(ctx, `SELECT `+jobColumns+` FROM import_job
		WHERE job_type=$1 AND source_version_id=$2 AND status='succeeded' LIMIT 1`, jobType, sourceVersionID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobNotFound
	}
	return job, err
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
		finished_at=NULL,error_json=NULL,updated_at=now() WHERE id=$1 AND status IN ('queued','failed','cancelled')`, jobID)
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
		error_json=$7::jsonb,finished_at=now(),updated_at=now() WHERE id=$1`,
		jobID, status, stage, progress, sourceVersionID, proposalID, errorJSON)
	return err
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
