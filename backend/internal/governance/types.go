// Package governance 实现 Proposal / Review / Conflict / ChangeBatch 治理边界（M5）。
// 机器与批量变更只能先形成 ProposalOperation，再由审核与 Apply 服务调用领域服务生效。
package governance

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrProposalNotFound      = errors.New("governance: Proposal 不存在")
	ErrInvalidActor          = errors.New("governance: 无效 Actor")
	ErrActorNotAllowed       = errors.New("governance: Actor 无权执行治理操作")
	ErrInvalidProposal       = errors.New("governance: Proposal 参数非法")
	ErrIdempotencyMismatch   = errors.New("governance: 幂等键对应的请求参数不一致")
	ErrInvalidTransition     = errors.New("governance: Proposal 状态转换非法")
	ErrProposalNotDraft      = errors.New("governance: Proposal 已提交，Operation 不可修改")
	ErrProposalHasNoOps      = errors.New("governance: Proposal 没有 Operation")
	ErrInvalidOperation      = errors.New("governance: Operation 参数非法")
	ErrOperationSequenceRace = errors.New("governance: Operation 序号冲突")
	ErrApprovalRequired      = errors.New("governance: 缺少有效审核批准证据")
	ErrRepairProjectionStale = errors.New("governance: 引用修复投影不是 Current Revision")
)

const (
	TargetPage             = "page"
	TargetEntity           = "entity"
	TargetClaim            = "claim"
	TargetCollection       = "collection"
	TargetExternalResource = "external_resource"

	ProposalDraft      = "draft"
	ProposalSubmitted  = "submitted"
	ProposalInReview   = "in_review"
	ProposalApproved   = "approved"
	ProposalRejected   = "rejected"
	ProposalConflicted = "conflicted"
	ProposalApplying   = "applying"
	ProposalApplied    = "applied"
	ProposalFailed     = "failed"
	ProposalRolledBack = "rolled_back"

	RiskLow      = "low"
	RiskMedium   = "medium"
	RiskHigh     = "high"
	RiskCritical = "critical"
)

var validTargets = map[string]bool{
	TargetPage: true, TargetEntity: true, TargetClaim: true,
	TargetCollection: true, TargetExternalResource: true,
}

var validRiskLevels = map[string]bool{
	RiskLow: true, RiskMedium: true, RiskHigh: true, RiskCritical: true,
}

var proposalTransitions = map[string]map[string]bool{
	ProposalDraft:      {ProposalSubmitted: true},
	ProposalSubmitted:  {ProposalInReview: true, ProposalApproved: true, ProposalRejected: true, ProposalConflicted: true},
	ProposalInReview:   {ProposalApproved: true, ProposalRejected: true, ProposalConflicted: true},
	ProposalApproved:   {ProposalApplying: true, ProposalConflicted: true},
	ProposalConflicted: {ProposalSubmitted: true, ProposalRejected: true},
	ProposalApplying:   {ProposalApplied: true, ProposalFailed: true},
	ProposalApplied:    {ProposalRolledBack: true},
}

// Proposal 是一组尚未正式生效的有序变更建议。
type Proposal struct {
	ID               uuid.UUID       `json:"id"`
	ImportJobID      *uuid.UUID      `json:"import_job_id"`
	TargetType       string          `json:"target_type"`
	TargetID         *uuid.UUID      `json:"target_id"`
	BaseRevisionID   *uuid.UUID      `json:"base_revision_id"`
	BaseStateVersion *int            `json:"base_state_version"`
	Status           string          `json:"status"`
	RiskLevel        string          `json:"risk_level"`
	RiskReasons      json.RawMessage `json:"risk_reasons"`
	PolicyDecision   json.RawMessage `json:"policy_decision"`
	CreatedBy        uuid.UUID       `json:"created_by"`
	IdempotencyKey   string          `json:"idempotency_key"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// OperationRecord 是 proposal_operation 的持久化形态；v1 语义类型见 operation.go。
type OperationRecord struct {
	ID             uuid.UUID
	ProposalID     uuid.UUID
	Sequence       int
	SchemaVersion  int
	OperationType  string
	TargetPageID   *uuid.UUID
	TargetBlockID  *string
	TargetNodeID   *string
	TargetEntityID *uuid.UUID
	TargetClaimID  *uuid.UUID
	Target         json.RawMessage
	ExpectedHash   *string
	Base           json.RawMessage
	Evidence       json.RawMessage
	Risk           json.RawMessage
	Payload        json.RawMessage
	CreatedAt      time.Time
}

// CreateProposalParams 以 (CreatedBy, IdempotencyKey) 幂等创建 Proposal。
type CreateProposalParams struct {
	ImportJobID      *uuid.UUID
	TargetType       string
	TargetID         *uuid.UUID
	BaseRevisionID   *uuid.UUID
	BaseStateVersion *int
	RiskLevel        string
	CreatedBy        uuid.UUID
	IdempotencyKey   string
}

// AddOperationParams 向 draft Proposal 末尾追加一条 Operation；Sequence 由服务端分配。
type AddOperationParams struct {
	ProposalID     uuid.UUID
	SchemaVersion  int
	OperationType  string
	TargetPageID   *uuid.UUID
	TargetBlockID  *string
	TargetNodeID   *string
	TargetEntityID *uuid.UUID
	TargetClaimID  *uuid.UUID
	Target         json.RawMessage
	ExpectedHash   *string
	Base           json.RawMessage
	Evidence       json.RawMessage
	Risk           json.RawMessage
	Payload        json.RawMessage
}

const (
	ReviewPending   = "pending"
	ReviewApproved  = "approved"
	ReviewRejected  = "rejected"
	ReviewCancelled = "cancelled"
)

type ReviewTask struct {
	ID             uuid.UUID  `json:"id"`
	ProposalID     uuid.UUID  `json:"proposal_id"`
	Status         string     `json:"status"`
	ReviewerID     *uuid.UUID `json:"reviewer_id"`
	DecisionReason *string    `json:"decision_reason"`
	CreatedAt      time.Time  `json:"created_at"`
	ReviewedAt     *time.Time `json:"reviewed_at"`
}

const (
	ConflictRevision   = "revision"
	ConflictBlockHash  = "block_hash"
	ConflictClaimState = "claim_state"
	ConflictSemantic   = "semantic"
	ConflictOpen       = "open"
	ConflictResolved   = "resolved"
	ConflictDismissed  = "dismissed"

	ResolutionChooseCurrent  = "choose_current"
	ResolutionChooseProposed = "choose_proposed"
	ResolutionDismiss        = "dismiss"
)

type MergeConflict struct {
	ID                uuid.UUID       `json:"id"`
	ProposalID        uuid.UUID       `json:"proposal_id"`
	PageID            *uuid.UUID      `json:"page_id"`
	ConflictType      string          `json:"conflict_type"`
	TargetBlockID     *string         `json:"target_block_id"`
	TargetClaimID     *uuid.UUID      `json:"target_claim_id"`
	BaseRevisionID    *uuid.UUID      `json:"base_revision_id"`
	CurrentRevisionID *uuid.UUID      `json:"current_revision_id"`
	BaseValue         json.RawMessage `json:"base_value"`
	CurrentValue      json.RawMessage `json:"current_value"`
	ProposedValue     json.RawMessage `json:"proposed_value"`
	Status            string          `json:"status"`
	ResolvedBy        *uuid.UUID      `json:"resolved_by"`
	Resolution        json.RawMessage `json:"resolution"`
	ResolvedAt        *time.Time      `json:"resolved_at"`
	CreatedAt         time.Time       `json:"created_at"`
}

const (
	BatchApplying        = "applying"
	BatchApplied         = "applied"
	BatchFailed          = "failed"
	BatchRollbackPending = "rollback_pending"
	BatchRolledBack      = "rolled_back"
)

type ChangeBatch struct {
	ID           uuid.UUID
	ImportJobID  *uuid.UUID
	ProposalID   uuid.UUID
	ActorID      uuid.UUID
	Status       string
	CreatedAt    time.Time
	RolledBackAt *time.Time
}

type BatchRevision struct {
	ID               uuid.UUID
	PageID           uuid.UUID
	ParentRevisionID *uuid.UUID
}
