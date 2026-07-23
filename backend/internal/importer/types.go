// Package importer 编排来源导入阶段；各阶段只调用 Evidence、AI、Knowledge 与
// Governance 领域服务，不直接写它们的权威表（M6）。
package importer

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrJobNotFound         = errors.New("importer: ImportJob 不存在")
	ErrRunNotFound         = errors.New("importer: ImportRun 不存在")
	ErrInvalidJob          = errors.New("importer: ImportJob 参数非法")
	ErrInvalidTransition   = errors.New("importer: 状态转换非法")
	ErrIdempotencyMismatch = errors.New("importer: 幂等键请求不一致")
	ErrCancelled           = errors.New("importer: 导入已取消")
	ErrQualityGate         = errors.New("importer: 质量门禁未通过")
	ErrAmbiguousEntity     = errors.New("importer: 实体匹配存在歧义")
	ErrEvidenceRequired    = errors.New("importer: 候选缺少可核验证据")
	ErrNoQueuedJob         = errors.New("importer: 没有排队中的导入任务")
)

const (
	JobQueued    = "queued"
	JobRunning   = "running"
	JobSucceeded = "succeeded"
	JobFailed    = "failed"
	JobCancelled = "cancelled"

	StageQueued   = "queued"
	StageFetch    = "fetch"
	StageParse    = "parse"
	StageExtract  = "extract"
	StageMatch    = "match"
	StageCompose  = "compose"
	StageReview   = "review"
	StageComplete = "complete"

	StageRunning   = "running"
	StageSucceeded = "succeeded"
	StageFailed    = "failed"
	StageSkipped   = "skipped"
	StageCancelled = "cancelled"
)

var stageProgress = map[string]int{
	StageFetch: 10, StageParse: 30, StageExtract: 55,
	StageMatch: 75, StageCompose: 90, StageReview: 95, StageComplete: 100,
}

type Job struct {
	ID              uuid.UUID       `json:"id"`
	JobType         string          `json:"job_type"`
	Status          string          `json:"status"`
	InitiatedBy     uuid.UUID       `json:"initiated_by"`
	IdempotencyKey  string          `json:"idempotency_key"`
	Config          json.RawMessage `json:"config"`
	SourceVersionID *uuid.UUID      `json:"source_version_id"`
	ProposalID      *uuid.UUID      `json:"proposal_id"`
	CurrentStage    string          `json:"current_stage"`
	Progress        int             `json:"progress"`
	Error           json.RawMessage `json:"error"`
	CreatedAt       time.Time       `json:"created_at"`
	StartedAt       *time.Time      `json:"started_at"`
	FinishedAt      *time.Time      `json:"finished_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Run struct {
	ID             uuid.UUID       `json:"id"`
	ImportJobID    uuid.UUID       `json:"import_job_id"`
	Attempt        int             `json:"attempt"`
	IdempotencyKey string          `json:"idempotency_key"`
	Status         string          `json:"status"`
	Error          json.RawMessage `json:"error"`
	StartedAt      time.Time       `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at"`
}

type StageRun struct {
	ID          uuid.UUID       `json:"id"`
	ImportRunID uuid.UUID       `json:"import_run_id"`
	Stage       string          `json:"stage"`
	Status      string          `json:"status"`
	InputHash   *string         `json:"input_hash"`
	OutputHash  *string         `json:"output_hash"`
	Error       json.RawMessage `json:"error"`
	StartedAt   time.Time       `json:"started_at"`
	FinishedAt  *time.Time      `json:"finished_at"`
}

type JobDetail struct {
	Job    *Job       `json:"job"`
	Runs   []Run      `json:"runs"`
	Stages []StageRun `json:"stages"`
}
