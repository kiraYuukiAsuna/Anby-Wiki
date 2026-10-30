package importer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type Service struct {
	repo *Repository
	txm  *db.TxManager
	ids  *id.Generator
}

type UploadIdentity struct {
	WikiID        uuid.UUID
	Filename      string
	MIMEType      string
	ContentHash   string
	IngestionMode string
	Title         string
	Instructions  string
	RouteMode     string
	PageID        *uuid.UUID
	AuthorActorID *uuid.UUID
}

const (
	DefaultPageSize = 20
	MaxPageSize     = 100
)

func NewService(repo *Repository, txm *db.TxManager, ids *id.Generator) *Service {
	return &Service{repo: repo, txm: txm, ids: ids}
}

func canonicalObject(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 {
		return json.RawMessage(`{}`), nil
	}
	var object map[string]any
	if err := json.Unmarshal(value, &object); err != nil || object == nil {
		return nil, ErrInvalidJob
	}
	return json.Marshal(object)
}

func (s *Service) Create(
	ctx context.Context,
	actorID, wikiID uuid.UUID,
	jobType, key string,
	config json.RawMessage,
) (*Job, error) {
	actorType, status, actorWikiID, err := s.repo.Actor(ctx, actorID)
	if err != nil || status != "active" || (actorType != "human" && actorType != "system") {
		return nil, ErrInvalidJob
	}
	if wikiID == uuid.Nil || actorType != "system" && actorWikiID != wikiID {
		return nil, ErrInvalidJob
	}
	jobType, key = strings.TrimSpace(jobType), strings.TrimSpace(key)
	if jobType == "" || key == "" {
		return nil, ErrInvalidJob
	}
	canonical, err := canonicalObject(config)
	if err != nil {
		return nil, err
	}
	planningInput, err := planningInputJSON(PlanningInput{})
	if err != nil {
		return nil, err
	}
	var identity struct {
		AuthorActorID *uuid.UUID `json:"author_actor_id"`
	}
	if err := json.Unmarshal(canonical, &identity); err != nil {
		return nil, ErrInvalidJob
	}
	authorID := identity.AuthorActorID
	if jobType == "source_import" {
		decoded, decodeErr := decodeSourceImportConfig(canonical)
		if decodeErr != nil {
			return nil, decodeErr
		}
		planningInput, err = planningInputJSON(PlanningInput{
			Title: decoded.Title, Instructions: decoded.Instructions,
			RouteMode: decoded.RouteMode, PageID: decoded.PageID,
			QualityThreshold: decoded.QualityThreshold,
		})
		if err != nil {
			return nil, err
		}
	}
	jobID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	job := &Job{ID: jobID, WikiID: wikiID, JobType: jobType, Status: JobQueued, InitiatedBy: actorID,
		IdempotencyKey: key, Config: canonical, PlanningInput: planningInput,
		CurrentStage: StageQueued}
	var inserted bool
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.checkAuthor(ctx, tx, actorID, wikiID, authorID); err != nil {
			return err
		}
		var err error
		inserted, err = s.repo.InsertJobIfAbsent(ctx, tx, job)
		return err
	})
	if err != nil {
		return nil, err
	}
	if inserted {
		return s.repo.GetJob(ctx, nil, job.ID)
	}
	existing, err := s.repo.GetJobByKey(ctx, actorID, key)
	if err != nil {
		return nil, err
	}
	if existing.WikiID != wikiID || existing.JobType != jobType ||
		!jsonEqual(existing.Config, canonical) {
		return nil, ErrIdempotencyMismatch
	}
	return existing, nil
}

// FindUploadByKey resolves a completed upload preflight without repeating its
// object-store or Asset side effects.
func (s *Service) FindUploadByKey(
	ctx context.Context,
	actorID uuid.UUID,
	key string,
	expected UploadIdentity,
) (*Job, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrInvalidJob
	}
	existing, err := s.repo.GetJobByKey(ctx, actorID, key)
	if err != nil {
		return nil, err
	}
	config, err := decodeSourceImportConfig(existing.Config)
	if err != nil {
		return nil, err
	}
	samePage := config.PageID == nil && expected.PageID == nil
	if config.PageID != nil && expected.PageID != nil {
		samePage = *config.PageID == *expected.PageID
	}
	sameAuthor := config.AuthorActorID == nil && expected.AuthorActorID == nil
	if config.AuthorActorID != nil && expected.AuthorActorID != nil {
		sameAuthor = *config.AuthorActorID == *expected.AuthorActorID
	}
	if existing.WikiID != expected.WikiID || existing.JobType != "source_import" ||
		config.Source.Kind != "upload" ||
		config.Source.Filename != expected.Filename ||
		config.Source.MIMEType != expected.MIMEType ||
		config.Source.ContentHash != expected.ContentHash ||
		config.IngestionMode != expected.IngestionMode ||
		config.Title != expected.Title ||
		config.Instructions != expected.Instructions ||
		config.RouteMode != expected.RouteMode ||
		!sameAuthor ||
		!samePage {
		return nil, ErrIdempotencyMismatch
	}
	return existing, nil
}

// ValidateAuthor is the upload preflight, before storing an Asset.
func (s *Service) ValidateAuthor(ctx context.Context, actorID, wikiID uuid.UUID, authorID *uuid.UUID) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		return s.checkAuthor(ctx, tx, actorID, wikiID, authorID)
	})
}

func (s *Service) checkAuthor(ctx context.Context, tx pgx.Tx, actorID, wikiID uuid.UUID, authorID *uuid.UUID) error {
	if authorID == nil {
		return nil
	}
	if err := authdomain.CheckMachineActorBinding(ctx, tx, *authorID, actorID, wikiID); err != nil {
		if errors.Is(err, authdomain.ErrUnauthenticated) || errors.Is(err, authdomain.ErrMachineActorForbidden) {
			return ErrAuthorUnavailable
		}
		return err
	}
	return nil
}

func (s *Service) checkJobAuthor(ctx context.Context, tx pgx.Tx, job *Job) error {
	if job.AuthorActorID == job.InitiatedBy {
		return nil
	}
	return s.checkAuthor(ctx, tx, job.InitiatedBy, job.WikiID, &job.AuthorActorID)
}

func (s *Service) ListOwned(
	ctx context.Context, actorID uuid.UUID, status, cursor string, limit int,
) (*JobPage, error) {
	actorType, actorStatus, _, err := s.repo.Actor(ctx, actorID)
	if err != nil || actorStatus != "active" || (actorType != "human" && actorType != "system") {
		return nil, ErrInvalidJob
	}
	status = strings.TrimSpace(status)
	if status != "" && status != JobQueued && status != JobRunning &&
		status != JobActionRequired && status != JobSucceeded && status != JobFailed &&
		status != JobCancelled {
		return nil, ErrInvalidStatus
	}
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	page, err := s.repo.ListOwnedJobs(ctx, actorID, status, cursor, limit)
	if err == nil {
		for index := range page.Items {
			page.Items[index].Error = enrichFailure(page.Items[index].Error)
		}
	}
	return page, err
}

func jsonEqual(left, right []byte) bool {
	var a, b any
	return json.Unmarshal(left, &a) == nil && json.Unmarshal(right, &b) == nil &&
		bytes.Equal(mustCanonicalJSON(a), mustCanonicalJSON(b))
}

func mustCanonicalJSON(value any) []byte {
	encoded, _ := json.Marshal(value)
	return encoded
}

func (s *Service) BeginRun(ctx context.Context, jobID uuid.UUID, key string) (*Run, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrInvalidJob
	}
	var result *Run
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if existing, err := s.repo.GetRunByKey(ctx, tx, jobID, key); err == nil {
			result = existing
			return nil
		} else if !errors.Is(err, ErrRunNotFound) {
			return err
		}
		if job.Status == JobSucceeded || job.Status == JobRunning {
			return ErrInvalidTransition
		}
		if err := s.repo.StartJob(ctx, tx, jobID); err != nil {
			return err
		}
		attempt, err := s.repo.NextAttempt(ctx, tx, jobID)
		if err != nil {
			return err
		}
		runID, err := s.ids.New()
		if err != nil {
			return err
		}
		result = &Run{ID: runID, ImportJobID: jobID, Attempt: attempt,
			IdempotencyKey: key, Status: JobRunning}
		return s.repo.InsertRun(ctx, tx, result)
	})
	return result, err
}

// ClaimNext atomically claims one queued source import and creates its Run.
// SKIP LOCKED lets multiple Worker processes share a queue without duplicate
// model calls. Pipeline.BeginRun later reuses the returned idempotency key.
func (s *Service) ClaimNext(ctx context.Context) (*Job, *Run, error) {
	return s.claimNext(ctx, false, nil)
}

func (s *Service) ClaimNextForWiki(
	ctx context.Context,
	wikiID uuid.UUID,
) (*Job, *Run, error) {
	if wikiID == uuid.Nil {
		return nil, nil, ErrInvalidJob
	}
	return s.claimNext(ctx, false, &wikiID)
}

// ClaimNextSourceOnly claims source-only work before consulting model runtime
// configuration. These jobs stop after immutable source parsing.
func (s *Service) ClaimNextSourceOnly(ctx context.Context) (*Job, *Run, error) {
	return s.claimNext(ctx, true, nil)
}

func (s *Service) NextQueuedAIWikiID(ctx context.Context) (uuid.UUID, error) {
	return s.repo.NextQueuedAIWikiID(ctx)
}

func (s *Service) QueuedAIWikiIDs(ctx context.Context, limit int) ([]uuid.UUID, error) {
	return s.repo.QueuedAIWikiIDs(ctx, limit)
}

func (s *Service) claimNext(
	ctx context.Context,
	sourceOnly bool,
	wikiID *uuid.UUID,
) (*Job, *Run, error) {
	var job *Job
	var run *Run
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		job, err = s.repo.NextQueuedJobForUpdate(ctx, tx, sourceOnly, wikiID)
		if err != nil {
			return err
		}
		if err := s.repo.StartJob(ctx, tx, job.ID); err != nil {
			return err
		}
		attempt, err := s.repo.NextAttempt(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		runID, err := s.ids.New()
		if err != nil {
			return err
		}
		run = &Run{ID: runID, ImportJobID: job.ID, Attempt: attempt,
			IdempotencyKey: fmt.Sprintf("worker:%s:attempt:%d", job.ID, attempt), Status: JobRunning}
		if err := s.repo.InsertRun(ctx, tx, run); err != nil {
			return err
		}
		job.Status = JobRunning
		job.StartedAt = &run.StartedAt
		return nil
	})
	return job, run, err
}

// RecoverStale requeues one run abandoned by a killed or restarted Worker.
// The cutoff must be at least the Runner's full job timeout, so a live Worker
// always loses its context before another process can reclaim the job.
func (s *Service) RecoverStale(ctx context.Context, staleBefore time.Time) (bool, error) {
	if staleBefore.IsZero() {
		return false, ErrInvalidJob
	}
	recovered := false
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.NextStaleRunningJobForUpdate(ctx, tx, staleBefore)
		if errors.Is(err, ErrNoQueuedJob) {
			return nil
		}
		if err != nil {
			return err
		}
		run, err := s.repo.RunningRun(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		errorJSON := safeError(job.CurrentStage, ErrorWorkerInterrupted)
		if err := s.repo.CancelRunningStages(ctx, tx, run.ID, errorJSON); err != nil {
			return err
		}
		if err := s.repo.FinishRun(ctx, tx, run.ID, JobFailed, errorJSON); err != nil {
			return err
		}
		if err := s.repo.FinishJob(ctx, tx, job.ID, JobFailed, job.CurrentStage,
			job.Progress, nil, nil, errorJSON); err != nil {
			return err
		}
		if err := s.repo.RequeueJob(ctx, tx, job.ID); err != nil {
			return err
		}
		recovered = true
		return nil
	})
	return recovered, err
}

func (s *Service) StartStage(ctx context.Context, runID uuid.UUID, stage string, inputHash *string) (*StageRun, error) {
	if _, ok := stageProgress[stage]; !ok || stage == StageComplete {
		return nil, ErrInvalidJob
	}
	id, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	stageRun := &StageRun{ID: id, ImportRunID: runID, Stage: stage,
		Status: StageRunning, InputHash: inputHash}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var jobID uuid.UUID
		var runStatus, jobStatus string
		if err := tx.QueryRow(ctx, `SELECT r.import_job_id,r.status,j.status FROM import_run r
			JOIN import_job j ON j.id=r.import_job_id WHERE r.id=$1 FOR UPDATE OF r,j`, runID).
			Scan(&jobID, &runStatus, &jobStatus); err != nil {
			return ErrRunNotFound
		}
		if runStatus != JobRunning || jobStatus != JobRunning {
			return ErrCancelled
		}
		job, err := s.repo.GetJob(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if err := s.checkJobAuthor(ctx, tx, job); err != nil {
			return err
		}
		if err := s.repo.InsertStage(ctx, tx, stageRun); err != nil {
			return err
		}
		return s.repo.AdvanceJob(ctx, tx, jobID, stage, stageProgress[stage])
	})
	return stageRun, err
}

func (s *Service) CompleteStage(ctx context.Context, jobID uuid.UUID, stageRun *StageRun, outputHash *string) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CompleteStage(ctx, tx, stageRun.ID, StageSucceeded, outputHash, nil); err != nil {
			return err
		}
		return s.repo.AdvanceJob(ctx, tx, jobID, stageRun.Stage, stageProgress[stageRun.Stage])
	})
}

func (s *Service) CompletePlanStage(
	ctx context.Context,
	jobID uuid.UUID,
	stageRun *StageRun,
	planID uuid.UUID,
	autoConfirm bool,
) error {
	if stageRun == nil || stageRun.Stage != StagePlan || planID == uuid.Nil {
		return ErrInvalidJob
	}
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		return s.repo.CompletePlanStage(
			ctx, tx, jobID, stageRun.ID, planID, autoConfirm,
		)
	})
}

// SkipStage records an intentional idempotency short-circuit while preserving
// a complete progress trail for the UI.
func (s *Service) SkipStage(ctx context.Context, jobID uuid.UUID, stageRun *StageRun, outputHash *string) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CompleteStage(ctx, tx, stageRun.ID, StageSkipped, outputHash, nil); err != nil {
			return err
		}
		return s.repo.AdvanceJob(ctx, tx, jobID, stageRun.Stage, stageProgress[stageRun.Stage])
	})
}

// CompleteParseStage atomically exposes the immutable parse checkpoint on the
// ImportJob. Later model or governance failures must not hide persisted source
// evidence from API clients or retry logic.
func (s *Service) CompleteParseStage(
	ctx context.Context,
	jobID uuid.UUID,
	stageRun *StageRun,
	sourceVersionID uuid.UUID,
	skipped bool,
) error {
	if stageRun == nil || stageRun.Stage != StageParse ||
		sourceVersionID == uuid.Nil {
		return ErrInvalidJob
	}
	status := StageSucceeded
	if skipped {
		status = StageSkipped
	}
	output := sourceVersionID.String()
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CompleteStage(
			ctx, tx, stageRun.ID, status, &output, nil,
		); err != nil {
			return err
		}
		return s.repo.AdvanceParsedJob(
			ctx, tx, jobID, sourceVersionID, stageProgress[StageParse],
		)
	})
}

func safeError(stage, code string) []byte {
	return newFailure(stage, code, nil).JSON()
}

func (s *Service) Fail(ctx context.Context, jobID, runID uuid.UUID, failure Failure) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.Status != JobRunning {
			// Cancellation, a recorded failure or a completed run wins over
			// late pipeline errors.
			return nil
		}
		run, err := s.repo.RunningRun(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if run.ID != runID {
			return nil
		}
		if failure.Stage == "" {
			failure.Stage = job.CurrentStage
		}
		errorJSON := failure.JSON()
		if err := s.repo.FailRunningStages(ctx, tx, runID, errorJSON); err != nil {
			return err
		}
		if err := s.repo.FinishRun(ctx, tx, runID, JobFailed, errorJSON); err != nil {
			return err
		}
		return s.repo.FinishJob(ctx, tx, jobID, JobFailed, failure.Stage,
			job.Progress, nil, nil, errorJSON)
	})
}

func (s *Service) Succeed(ctx context.Context, jobID, runID uuid.UUID, sourceVersionID uuid.UUID, proposalID *uuid.UUID) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.Status != JobRunning {
			return ErrInvalidTransition
		}
		if err := s.repo.FinishRun(ctx, tx, runID, JobSucceeded, nil); err != nil {
			return err
		}
		return s.repo.FinishJob(ctx, tx, jobID, JobSucceeded, StageComplete, 100,
			&sourceVersionID, proposalID, nil)
	})
}

// SucceedReused completes a duplicate submission by linking the canonical
// proposal. CompleteParseStage has already linked the reused SourceVersion.
func (s *Service) SucceedReused(ctx context.Context, jobID, runID uuid.UUID, proposalID *uuid.UUID) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.Status != JobRunning {
			return ErrInvalidTransition
		}
		if err := s.repo.FinishRun(ctx, tx, runID, JobSucceeded, nil); err != nil {
			return err
		}
		return s.repo.FinishJob(ctx, tx, jobID, JobSucceeded, StageComplete, 100, nil, proposalID, nil)
	})
}

func (s *Service) RequireAction(
	ctx context.Context,
	jobID, runID, sourceVersionID uuid.UUID,
	action string,
) error {
	return s.RequireActionWithDetails(ctx, jobID, runID, sourceVersionID, action, nil)
}

func (s *Service) RequireActionWithDetails(
	ctx context.Context,
	jobID, runID, sourceVersionID uuid.UUID,
	action string,
	errorJSON []byte,
) error {
	if action != ActionConfirmPlan && action != ActionQualityGate && action != ActionResolveEntities {
		return ErrInvalidJob
	}
	stage := StagePlan
	if action == ActionResolveEntities {
		stage = StageMatch
	}
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.Status != JobRunning {
			return ErrInvalidTransition
		}
		if job.CurrentPlanID == nil {
			return ErrImportPlanNotFound
		}
		if _, err := s.repo.GetImportPlanByID(ctx, jobID, *job.CurrentPlanID); err != nil {
			return err
		}
		if err := s.repo.FinishRun(ctx, tx, runID, JobSucceeded, nil); err != nil {
			return err
		}
		return s.repo.RequireAction(ctx, tx, jobID, sourceVersionID, action, stage, errorJSON)
	})
}

func (s *Service) ConfirmPlan(ctx context.Context, jobID, planID uuid.UUID) error {
	plan, err := s.repo.GetImportPlanByID(ctx, jobID, planID)
	if err != nil {
		return err
	}
	var document ImportPlan
	if err := json.Unmarshal(plan.PlanJSON, &document); err != nil {
		return err
	}
	if document.Quality == nil || !document.Quality.Passed {
		return ErrQualityGate
	}
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.CurrentPlanID == nil || *job.CurrentPlanID != planID {
			return ErrInvalidTransition
		}
		if job.ConfirmedPlanID != nil && *job.ConfirmedPlanID == planID {
			return nil
		}
		if job.Status != JobActionRequired ||
			job.ActionRequired == nil || *job.ActionRequired != ActionConfirmPlan {
			return ErrInvalidTransition
		}
		if err := s.checkJobAuthor(ctx, tx, job); err != nil {
			return err
		}
		return s.repo.ConfirmPlan(ctx, tx, jobID, plan.ID)
	})
}

func (s *Service) Replan(
	ctx context.Context,
	actorID, jobID uuid.UUID,
	key string,
	input ReplanInput,
) (*Job, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrInvalidJob
	}
	var result *Job
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.InitiatedBy != actorID {
			return ErrInvalidJob
		}
		if err := s.checkJobAuthor(ctx, tx, job); err != nil {
			return err
		}
		previous, err := decodePlanningInput(job.PlanningInput)
		if err != nil {
			return err
		}
		next := PlanningInput{
			Title:        strings.TrimSpace(input.Title),
			Instructions: strings.TrimSpace(input.Instructions),
			RouteMode:    strings.TrimSpace(input.RouteMode),
			PageID:       input.PageID, QualityThreshold: previous.QualityThreshold,
		}
		raw, err := planningInputJSON(next)
		if err != nil {
			return err
		}
		if job.PlanningIdempotencyKey != nil && *job.PlanningIdempotencyKey == key {
			if !jsonEqual(job.PlanningInput, raw) {
				return ErrIdempotencyMismatch
			}
			result = job
			return nil
		}
		hasProposal, err := s.repo.HasProposal(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		if job.SourceVersionID == nil || job.ProposalID != nil || hasProposal {
			return ErrInvalidTransition
		}
		if job.Status == JobQueued || job.Status == JobRunning {
			return ErrInvalidTransition
		}
		if err := s.repo.RequeueForPlanning(ctx, tx, job.ID, raw, key); err != nil {
			return err
		}
		job.Status = JobQueued
		job.PlanningInput = raw
		job.PlanningIdempotencyKey = &key
		job.ActionRequired = nil
		job.ConfirmedPlanID = nil
		job.PlanConfirmedAt = nil
		job.CurrentStage = StageQueued
		job.Progress = 0
		job.Error = nil
		job.FinishedAt = nil
		result = job
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Service) Cancel(ctx context.Context, jobID uuid.UUID) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.Status == JobCancelled {
			return nil
		}
		if job.Status != JobQueued && job.Status != JobRunning &&
			job.Status != JobActionRequired {
			return ErrInvalidTransition
		}
		if job.Status == JobRunning {
			if run, err := s.repo.RunningRun(ctx, tx, jobID); err == nil {
				errorJSON := safeError(job.CurrentStage, ErrorCancelled)
				if err := s.repo.CancelRunningStages(ctx, tx, run.ID, errorJSON); err != nil {
					return err
				}
				if err := s.repo.FinishRun(ctx, tx, run.ID, JobCancelled, errorJSON); err != nil {
					return err
				}
			}
		}
		return s.repo.FinishJob(ctx, tx, jobID, JobCancelled, job.CurrentStage,
			job.Progress, nil, nil, safeError(job.CurrentStage, ErrorCancelled))
	})
}

func (s *Service) Retry(ctx context.Context, jobID uuid.UUID) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if err := s.checkJobAuthor(ctx, tx, job); err != nil {
			return err
		}
		return s.repo.RequeueJob(ctx, tx, jobID)
	})
}

func (s *Service) Detail(ctx context.Context, jobID uuid.UUID) (*JobDetail, error) {
	job, err := s.repo.GetJob(ctx, nil, jobID)
	if err != nil {
		return nil, err
	}
	runs, err := s.repo.ListRuns(ctx, jobID)
	if err != nil {
		return nil, err
	}
	stages, err := s.repo.ListStages(ctx, jobID)
	if err != nil {
		return nil, err
	}
	records, err := s.repo.ListImportPlans(ctx, jobID)
	if err != nil {
		return nil, err
	}
	plans := make([]PlanVersion, 0, len(records))
	var current *ImportPlan
	for index := range records {
		record := records[index]
		plans = append(plans, PlanVersion{
			ID: record.ID, Revision: record.Revision,
			ParentPlanID:  record.ParentPlanID,
			PlanningInput: record.PlanningInput, QualityScore: record.QualityScore,
			CreatedBy: record.CreatedBy, CreatedAt: record.CreatedAt,
		})
		if job.CurrentPlanID != nil && record.ID == *job.CurrentPlanID {
			document := &ImportPlan{}
			if err := json.Unmarshal(record.PlanJSON, document); err != nil {
				return nil, err
			}
			normalizeImportPlanCollections(document)
			current = document
		}
	}
	if current == nil && len(plans) > 0 {
		current = &ImportPlan{}
		if err := json.Unmarshal(records[len(records)-1].PlanJSON, current); err != nil {
			return nil, err
		}
		normalizeImportPlanCollections(current)
	}
	detail := &JobDetail{
		Job: job, Runs: runs, Stages: stages, Plan: current, Plans: plans,
		MatchReview: matchReviewFromJob(job),
	}
	if err := s.enrichDetailFailures(ctx, detail); err != nil {
		return nil, err
	}
	return detail, nil
}

func matchReviewFromJob(job *Job) *MatchReview {
	if job == nil || job.ActionRequired == nil || *job.ActionRequired != ActionResolveEntities ||
		len(job.Error) == 0 {
		return nil
	}
	var payload struct {
		MatchReview *MatchReview `json:"match_review"`
	}
	if json.Unmarshal(job.Error, &payload) != nil {
		return nil
	}
	return payload.MatchReview
}

func HashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func WrapStageError(stage string, err error) error {
	return fmt.Errorf("importer: %s: %w", stage, err)
}

func planningInputJSON(input PlanningInput) (json.RawMessage, error) {
	if input.RouteMode == "" {
		input.RouteMode = RouteModeAuto
	}
	if input.RouteMode != RouteModeAuto && input.RouteMode != RouteModeForceCreate &&
		input.RouteMode != RouteModeForceUpdate {
		return nil, ErrInvalidJob
	}
	input.Title = strings.TrimSpace(input.Title)
	input.Instructions = strings.TrimSpace(input.Instructions)
	if len([]rune(input.Title)) > 255 || len([]rune(input.Instructions)) > 4000 ||
		input.QualityThreshold < 0 || input.QualityThreshold > 1 ||
		(input.RouteMode == RouteModeForceCreate && input.Title == "") ||
		(input.RouteMode == RouteModeForceUpdate &&
			(input.PageID == nil || *input.PageID == uuid.Nil)) {
		return nil, ErrInvalidJob
	}
	return json.Marshal(input)
}

func decodePlanningInput(raw json.RawMessage) (*PlanningInput, error) {
	var input PlanningInput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, ErrInvalidJob
	}
	canonical, err := planningInputJSON(input)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(canonical, &input); err != nil {
		return nil, err
	}
	return &input, nil
}
