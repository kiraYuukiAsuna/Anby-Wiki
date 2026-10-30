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
	ErrInvalidCursor       = errors.New("importer: cursor 非法")
	ErrInvalidStatus       = errors.New("importer: status 非法")
	ErrInvalidTransition   = errors.New("importer: 状态转换非法")
	ErrIdempotencyMismatch = errors.New("importer: 幂等键请求不一致")
	ErrCancelled           = errors.New("importer: 导入已取消")
	ErrQualityGate         = errors.New("importer: 质量门禁未通过")
	ErrAmbiguousEntity     = errors.New("importer: 实体匹配存在歧义")
	ErrEvidenceRequired    = errors.New("importer: 候选缺少可核验证据")
	ErrModelInputTooLarge  = errors.New("importer: 结构化内容超过模型窗口预算")
	ErrNoQueuedJob         = errors.New("importer: 没有排队中的导入任务")
	ErrSourceOnlyFormat    = errors.New("importer: source_only 仅支持确定性解析格式")
	ErrAuthorUnavailable   = errors.New("importer: 机器作者不可用或不属于当前账户及 Wiki")
)

const (
	JobQueued         = "queued"
	JobRunning        = "running"
	JobActionRequired = "action_required"
	JobSucceeded      = "succeeded"
	JobFailed         = "failed"
	JobCancelled      = "cancelled"

	ActionConfirmPlan     = "confirm_plan"
	ActionQualityGate     = "quality_gate"
	ActionResolveEntities = "resolve_entities"

	IngestionModeWiki       = "wiki"
	IngestionModeSourceOnly = "source_only"
	ErrorSourceOnlyFormat   = "source_only_unsupported_format"

	StageQueued   = "queued"
	StageFetch    = "fetch"
	StageParse    = "parse"
	StageExtract  = "extract"
	StagePlan     = "plan"
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
	StageFetch: 8, StageParse: 22, StageExtract: 42, StagePlan: 65,
	StageMatch: 78, StageCompose: 90, StageReview: 96, StageComplete: 100,
}

type Job struct {
	ID                     uuid.UUID       `json:"id"`
	WikiID                 uuid.UUID       `json:"wiki_id"`
	JobType                string          `json:"job_type"`
	Status                 string          `json:"status"`
	InitiatedBy            uuid.UUID       `json:"initiated_by"`
	AuthorActorID          uuid.UUID       `json:"author_actor_id"`
	AuthorDisplayName      string          `json:"author_display_name"`
	IdempotencyKey         string          `json:"idempotency_key"`
	Config                 json.RawMessage `json:"config"`
	PlanningInput          json.RawMessage `json:"planning_input"`
	PlanningIdempotencyKey *string         `json:"-"`
	SourceVersionID        *uuid.UUID      `json:"source_version_id"`
	ProposalID             *uuid.UUID      `json:"proposal_id"`
	ActionRequired         *string         `json:"action_required"`
	CurrentPlanID          *uuid.UUID      `json:"current_plan_id"`
	ConfirmedPlanID        *uuid.UUID      `json:"confirmed_plan_id"`
	PlanConfirmedAt        *time.Time      `json:"plan_confirmed_at"`
	CurrentStage           string          `json:"current_stage"`
	Progress               int             `json:"progress"`
	Error                  json.RawMessage `json:"error"`
	CreatedAt              time.Time       `json:"created_at"`
	StartedAt              *time.Time      `json:"started_at"`
	FinishedAt             *time.Time      `json:"finished_at"`
	UpdatedAt              time.Time       `json:"updated_at"`
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
	Job         *Job          `json:"job"`
	Runs        []Run         `json:"runs"`
	Stages      []StageRun    `json:"stages"`
	Plan        *ImportPlan   `json:"plan"`
	Plans       []PlanVersion `json:"plans"`
	MatchReview *MatchReview  `json:"match_review,omitempty"`
}

type MatchReview struct {
	Resolutions   []EntityResolution   `json:"resolutions"`
	Candidates    []EntityMatchReview  `json:"candidates"`
	SkippedClaims []SkippedClaimReview `json:"skipped_claims"`
}

type EntityMatchReview struct {
	CandidateID     uuid.UUID           `json:"candidate_id"`
	TypeKey         string              `json:"type_key"`
	Label           string              `json:"label"`
	Aliases         []string            `json:"aliases"`
	Outcome         string              `json:"outcome"`
	EntityID        *uuid.UUID          `json:"entity_id,omitempty"`
	PlannedEntityID *uuid.UUID          `json:"planned_entity_id,omitempty"`
	Score           float64             `json:"score"`
	Reason          string              `json:"reason"`
	Alternatives    []EntityAlternative `json:"alternatives"`
}

type SkippedClaimReview struct {
	CandidateID uuid.UUID `json:"candidate_id"`
	Reason      string    `json:"reason"`
}

type JobPage struct {
	Items      []Job   `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

type ReplanInput struct {
	Title        string
	Instructions string
	RouteMode    string
	PageID       *uuid.UUID
}

type PlanningInput struct {
	Title            string     `json:"title,omitempty"`
	Instructions     string     `json:"instructions,omitempty"`
	RouteMode        string     `json:"route_mode"`
	PageID           *uuid.UUID `json:"page_id,omitempty"`
	QualityThreshold float64    `json:"quality_threshold,omitempty"`
}
