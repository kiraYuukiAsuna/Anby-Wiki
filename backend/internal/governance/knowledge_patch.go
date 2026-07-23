package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/knowledge"
)

// KnowledgePatchEngine 不持有 Repository；所有权威写入都转交 M4 knowledge.Service，
// 因而不能绕过值类型、Claim 状态机、Supersede 链与 Citation 存在性校验。
type KnowledgePatchEngine struct{ knowledge *knowledge.Service }

func NewKnowledgePatchEngine(service *knowledge.Service) *KnowledgePatchEngine {
	return &KnowledgePatchEngine{knowledge: service}
}

type KnowledgePatchResult struct {
	OperationType string
	EntityID      *uuid.UUID
	ClaimID       *uuid.UUID
	CitationID    *uuid.UUID
}

// ApplyOne 只接受四种知识写操作。actorID 是通过审核后实际执行的 human/system/bot
// Actor；AI Actor 会被 knowledge.Service 的统一准入规则拒绝（INV-05）。
func (e *KnowledgePatchEngine) ApplyOne(ctx context.Context, op OperationV1, actorID uuid.UUID, changeBatchID *uuid.UUID) (*KnowledgePatchResult, error) {
	return e.applyOne(ctx, nil, op, actorID, changeBatchID)
}

// ApplyOneInTx 由 Proposal Apply 调用，跨 Operation/ChangeBatch 保持单事务。
func (e *KnowledgePatchEngine) ApplyOneInTx(ctx context.Context, tx pgx.Tx, op OperationV1, actorID uuid.UUID, changeBatchID *uuid.UUID) (*KnowledgePatchResult, error) {
	return e.applyOne(ctx, tx, op, actorID, changeBatchID)
}

func (e *KnowledgePatchEngine) PublishAppliedClaimInTx(ctx context.Context, tx pgx.Tx, claimID uuid.UUID) error {
	_, err := e.knowledge.PublishClaimInTx(ctx, tx, claimID)
	return err
}

func (e *KnowledgePatchEngine) RollbackClaimInTx(ctx context.Context, tx pgx.Tx, claimID, actorID, batchID uuid.UUID) (*uuid.UUID, error) {
	claim, err := e.knowledge.RollbackClaimInTx(ctx, tx, claimID, actorID, &batchID)
	if err != nil {
		return nil, err
	}
	return &claim.ID, nil
}

func (e *KnowledgePatchEngine) applyOne(ctx context.Context, tx pgx.Tx, op OperationV1, actorID uuid.UUID, changeBatchID *uuid.UUID) (*KnowledgePatchResult, error) {
	if e == nil || e.knowledge == nil {
		return nil, fmt.Errorf("%w: knowledge service 未装配", ErrInvalidOperation)
	}
	switch op.OperationType {
	case OpCreateEntity:
		if op.Target.WikiID == nil {
			return nil, ErrInvalidOperation
		}
		var p struct {
			TypeKey      string `json:"type_key"`
			CanonicalKey string `json:"canonical_key"`
			Labels       []struct {
				Language    string `json:"language"`
				Label       string `json:"label"`
				Description string `json:"description"`
				IsPrimary   bool   `json:"is_primary"`
			} `json:"labels"`
		}
		if err := decodePayload(&op, &p); err != nil {
			return nil, err
		}
		labels := make([]knowledge.LabelInput, len(p.Labels))
		for i, label := range p.Labels {
			labels[i] = knowledge.LabelInput{Language: label.Language, Label: label.Label,
				Description: label.Description, IsPrimary: label.IsPrimary}
		}
		entityParams := knowledge.CreateEntityParams{
			WikiID: *op.Target.WikiID, TypeKey: p.TypeKey, CanonicalKey: p.CanonicalKey,
			Labels: labels, ActorID: actorID,
		}
		var entity *knowledge.Entity
		var err error
		if tx != nil {
			entity, err = e.knowledge.CreateEntityInTx(ctx, tx, entityParams)
		} else {
			entity, err = e.knowledge.CreateEntity(ctx, entityParams)
		}
		if err != nil {
			return nil, err
		}
		return &KnowledgePatchResult{OperationType: op.OperationType, EntityID: &entity.ID}, nil

	case OpCreateClaim:
		if op.Target.EntityID == nil {
			return nil, ErrInvalidOperation
		}
		params, err := decodeClaimPayload(op.Payload)
		if err != nil {
			return nil, err
		}
		claimParams := knowledge.CreateClaimParams{
			SubjectEntityID: *op.Target.EntityID, PropertyKey: params.PropertyKey,
			Value: params.Value, Qualifiers: params.Qualifiers, Rank: params.Rank,
			ValidFrom: params.ValidFrom, ValidTo: params.ValidTo,
			OriginType: params.OriginType, ActorID: actorID, ChangeBatchID: changeBatchID,
		}
		var claim *knowledge.Claim
		if tx != nil {
			claim, err = e.knowledge.CreateClaimInTx(ctx, tx, claimParams)
		} else {
			claim, err = e.knowledge.CreateClaim(ctx, claimParams)
		}
		if err != nil {
			return nil, err
		}
		if err := e.attachClaimSources(ctx, tx, claim.ID, params.CitationIDs); err != nil {
			return nil, err
		}
		return &KnowledgePatchResult{OperationType: op.OperationType, ClaimID: &claim.ID}, nil

	case OpSupersedeClaim:
		if op.Target.ClaimID == nil {
			return nil, ErrInvalidOperation
		}
		var envelope struct {
			SubjectEntityID uuid.UUID `json:"subject_entity_id"`
		}
		if err := json.Unmarshal(op.Payload, &envelope); err != nil || envelope.SubjectEntityID == uuid.Nil {
			return nil, ErrInvalidOperation
		}
		params, err := decodeClaimPayload(op.Payload)
		if err != nil {
			return nil, err
		}
		supersedeParams := knowledge.SupersedeClaimParams{
			ClaimID: *op.Target.ClaimID, SubjectEntityID: envelope.SubjectEntityID,
			PropertyKey: params.PropertyKey, Value: params.Value,
			Qualifiers: params.Qualifiers, Rank: params.Rank,
			ValidFrom: params.ValidFrom, ValidTo: params.ValidTo,
			OriginType: params.OriginType, ActorID: actorID, ChangeBatchID: changeBatchID,
		}
		var claim *knowledge.Claim
		if tx != nil {
			claim, err = e.knowledge.SupersedeClaimInTx(ctx, tx, supersedeParams)
		} else {
			claim, err = e.knowledge.SupersedeClaim(ctx, supersedeParams)
		}
		if err != nil {
			return nil, err
		}
		if err := e.attachClaimSources(ctx, tx, claim.ID, params.CitationIDs); err != nil {
			return nil, err
		}
		return &KnowledgePatchResult{OperationType: op.OperationType, ClaimID: &claim.ID}, nil

	case OpAddClaimSource:
		if op.Target.ClaimID == nil || op.Target.CitationID == nil {
			return nil, ErrInvalidOperation
		}
		var p struct {
			SupportType string `json:"support_type"`
		}
		if err := decodePayload(&op, &p); err != nil {
			return nil, err
		}
		sourceParams := knowledge.AddClaimSourceParams{
			ClaimID: *op.Target.ClaimID, CitationID: *op.Target.CitationID, SupportType: p.SupportType,
		}
		var source *knowledge.ClaimSource
		var err error
		if tx != nil {
			source, err = e.knowledge.AddClaimSourceInTx(ctx, tx, sourceParams)
		} else {
			source, err = e.knowledge.AddClaimSource(ctx, sourceParams)
		}
		if err != nil {
			return nil, err
		}
		return &KnowledgePatchResult{OperationType: op.OperationType,
			ClaimID: &source.ClaimID, CitationID: &source.CitationID}, nil
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedOperation, op.OperationType)
	}
}

type decodedClaimPayload struct {
	PropertyKey string
	Value       knowledge.Value
	Qualifiers  json.RawMessage
	Rank        string
	ValidFrom   *time.Time
	ValidTo     *time.Time
	OriginType  string
	CitationIDs []uuid.UUID
}

func decodeClaimPayload(raw json.RawMessage) (*decodedClaimPayload, error) {
	var p struct {
		PropertyKey string          `json:"property_key"`
		Value       json.RawMessage `json:"value"`
		Qualifiers  json.RawMessage `json:"qualifiers"`
		Rank        string          `json:"rank"`
		ValidFrom   *time.Time      `json:"valid_from"`
		ValidTo     *time.Time      `json:"valid_to"`
		OriginType  string          `json:"origin_type"`
		CitationIDs []uuid.UUID     `json:"citation_ids"`
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.PropertyKey == "" || len(p.Value) == 0 {
		return nil, ErrInvalidOperation
	}
	value, err := decodeKnowledgeValue(p.Value)
	if err != nil {
		return nil, err
	}
	return &decodedClaimPayload{
		PropertyKey: p.PropertyKey, Value: value, Qualifiers: p.Qualifiers, Rank: p.Rank,
		ValidFrom: p.ValidFrom, ValidTo: p.ValidTo, OriginType: p.OriginType,
		CitationIDs: p.CitationIDs,
	}, nil
}

func (e *KnowledgePatchEngine) attachClaimSources(ctx context.Context, tx pgx.Tx, claimID uuid.UUID, citationIDs []uuid.UUID) error {
	seen := make(map[uuid.UUID]bool, len(citationIDs))
	for _, citationID := range citationIDs {
		if citationID == uuid.Nil || seen[citationID] {
			return ErrInvalidOperation
		}
		seen[citationID] = true
		params := knowledge.AddClaimSourceParams{ClaimID: claimID, CitationID: citationID, SupportType: knowledge.SupportTypeSupports}
		var err error
		if tx != nil {
			_, err = e.knowledge.AddClaimSourceInTx(ctx, tx, params)
		} else {
			_, err = e.knowledge.AddClaimSource(ctx, params)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func decodeKnowledgeValue(raw json.RawMessage) (knowledge.Value, error) {
	var v struct {
		String     *string               `json:"string"`
		Number     *float64              `json:"number"`
		Date       *string               `json:"date"`
		EntityID   *uuid.UUID            `json:"entity_id"`
		Coordinate *knowledge.Coordinate `json:"coordinate"`
		Composite  json.RawMessage       `json:"composite"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return knowledge.Value{}, fmt.Errorf("%w: value: %v", ErrInvalidOperation, err)
	}
	set := 0
	if v.String != nil {
		set++
	}
	if v.Number != nil {
		set++
	}
	if v.Date != nil {
		set++
	}
	if v.EntityID != nil {
		set++
	}
	if v.Coordinate != nil {
		set++
	}
	if len(v.Composite) > 0 {
		set++
	}
	if set != 1 {
		return knowledge.Value{}, fmt.Errorf("%w: value 必须且只能设置一种形态", ErrInvalidOperation)
	}
	return knowledge.Value{
		String: v.String, Number: v.Number, Date: v.Date, EntityID: v.EntityID,
		Coordinate: v.Coordinate, Composite: v.Composite,
	}, nil
}
