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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type Service struct {
	repo *Repository
	txm  *db.TxManager
	ids  *id.Generator
}

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

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, jobType, key string, config json.RawMessage) (*Job, error) {
	actorType, status, err := s.repo.Actor(ctx, actorID)
	if err != nil || status != "active" || (actorType != "human" && actorType != "system") {
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
	jobID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	job := &Job{ID: jobID, JobType: jobType, Status: JobQueued, InitiatedBy: actorID,
		IdempotencyKey: key, Config: canonical, CurrentStage: StageQueued}
	inserted, err := s.repo.InsertJobIfAbsent(ctx, job)
	if err != nil {
		return nil, err
	}
	if inserted {
		return job, nil
	}
	existing, err := s.repo.GetJobByKey(ctx, actorID, key)
	if err != nil {
		return nil, err
	}
	if existing.JobType != jobType || !jsonEqual(existing.Config, canonical) {
		return nil, ErrIdempotencyMismatch
	}
	return existing, nil
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
	var job *Job
	var run *Run
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var err error
		job, err = s.repo.NextQueuedJobForUpdate(ctx, tx)
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

func safeError(stage, code string) []byte {
	value, _ := json.Marshal(map[string]string{"stage": stage, "code": code})
	return value
}

func (s *Service) Fail(ctx context.Context, jobID, runID uuid.UUID, stageRun *StageRun, code string) error {
	errorJSON := safeError(stageRun.Stage, code)
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.repo.CompleteStage(ctx, tx, stageRun.ID, StageFailed, nil, errorJSON); err != nil {
			return err
		}
		if err := s.repo.FinishRun(ctx, tx, runID, JobFailed, errorJSON); err != nil {
			return err
		}
		return s.repo.FinishJob(ctx, tx, jobID, JobFailed, stageRun.Stage,
			stageProgress[stageRun.Stage], nil, nil, errorJSON)
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
// proposal without claiming a second successful owner for source_version_id.
// The skipped parse stage retains the reused version ID in its output hash.
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

func (s *Service) Cancel(ctx context.Context, jobID uuid.UUID) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		job, err := s.repo.GetJobForUpdate(ctx, tx, jobID)
		if err != nil {
			return err
		}
		if job.Status == JobCancelled {
			return nil
		}
		if job.Status != JobQueued && job.Status != JobRunning {
			return ErrInvalidTransition
		}
		if job.Status == JobRunning {
			if run, err := s.repo.RunningRun(ctx, tx, jobID); err == nil {
				errorJSON := safeError(job.CurrentStage, "cancelled")
				if err := s.repo.CancelRunningStages(ctx, tx, run.ID, errorJSON); err != nil {
					return err
				}
				if err := s.repo.FinishRun(ctx, tx, run.ID, JobCancelled, errorJSON); err != nil {
					return err
				}
			}
		}
		return s.repo.FinishJob(ctx, tx, jobID, JobCancelled, job.CurrentStage,
			job.Progress, nil, nil, safeError(job.CurrentStage, "cancelled"))
	})
}

func (s *Service) Retry(ctx context.Context, jobID uuid.UUID) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := s.repo.GetJobForUpdate(ctx, tx, jobID); err != nil {
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
	return &JobDetail{Job: job, Runs: runs, Stages: stages}, nil
}

func HashBytes(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func WrapStageError(stage string, err error) error {
	return fmt.Errorf("importer: %s: %w", stage, err)
}
