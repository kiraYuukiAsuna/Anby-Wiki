package governance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
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

// CreateProposal 允许 active human/bot/ai/import/system 创建提案；anonymous 不允许。
// 此入口只写治理草稿，不触碰 Revision、Claim 或 Projection。
func (s *Service) CreateProposal(ctx context.Context, in CreateProposalParams) (*Proposal, error) {
	in.IdempotencyKey = strings.TrimSpace(in.IdempotencyKey)
	if !validTargets[in.TargetType] || in.CreatedBy == uuid.Nil || in.IdempotencyKey == "" || len(in.IdempotencyKey) > 200 {
		return nil, ErrInvalidProposal
	}
	if in.BaseStateVersion != nil && *in.BaseStateVersion < 0 {
		return nil, ErrInvalidProposal
	}
	if in.RiskLevel == "" {
		in.RiskLevel = RiskLow
	}
	if !validRiskLevels[in.RiskLevel] {
		return nil, ErrInvalidProposal
	}
	if err := s.checkProposalActor(ctx, nil, in.CreatedBy); err != nil {
		return nil, err
	}

	var result *Proposal
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		proposalID, err := s.ids.New()
		if err != nil {
			return err
		}
		candidate := &Proposal{
			ID: proposalID, ImportJobID: in.ImportJobID, TargetType: in.TargetType,
			TargetID: in.TargetID, BaseRevisionID: in.BaseRevisionID,
			BaseStateVersion: in.BaseStateVersion, Status: ProposalDraft,
			RiskLevel: in.RiskLevel, CreatedBy: in.CreatedBy, IdempotencyKey: in.IdempotencyKey,
		}
		inserted, err := s.repo.InsertProposal(ctx, tx, candidate)
		if err != nil {
			return err
		}
		if !inserted {
			existing, err := s.repo.GetProposalByIdempotency(ctx, tx, in.CreatedBy, in.IdempotencyKey)
			if err != nil {
				return err
			}
			if !sameProposalRequest(existing, in) {
				return ErrIdempotencyMismatch
			}
			result = existing
			return nil
		}
		result, err = s.repo.GetProposal(ctx, tx, proposalID)
		return err
	})
	return result, err
}

func sameProposalRequest(p *Proposal, in CreateProposalParams) bool {
	return p.TargetType == in.TargetType && equalUUID(p.ImportJobID, in.ImportJobID) &&
		equalUUID(p.TargetID, in.TargetID) && equalUUID(p.BaseRevisionID, in.BaseRevisionID) &&
		reflect.DeepEqual(p.BaseStateVersion, in.BaseStateVersion) &&
		// risk_level is replaced by the policy evaluator at Submit. Once frozen,
		// compare the immutable request identity rather than that derived field.
		(p.Status != ProposalDraft || p.RiskLevel == in.RiskLevel)
}

func equalUUID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// AddOperation 在 Proposal 行锁内分配严格连续的 1-based sequence。
func (s *Service) AddOperation(ctx context.Context, in AddOperationParams) (*OperationRecord, error) {
	if in.ProposalID == uuid.Nil || strings.TrimSpace(in.OperationType) == "" {
		return nil, ErrInvalidOperation
	}
	if in.SchemaVersion == 0 {
		in.SchemaVersion = 1
	}
	if in.SchemaVersion != 1 || !isJSONObject(in.Payload) {
		return nil, ErrInvalidOperation
	}
	if len(in.Evidence) == 0 {
		in.Evidence = json.RawMessage(`[]`)
	}
	if len(in.Base) == 0 {
		in.Base = json.RawMessage(`{}`)
	}
	if len(in.Target) == 0 {
		in.Target = json.RawMessage(`{}`)
	}
	if len(in.Risk) == 0 {
		in.Risk = json.RawMessage(`{"level":"low","reasons":[]}`)
	}
	if !isJSONObject(in.Base) || !isJSONObject(in.Target) || !isJSONArray(in.Evidence) || !isJSONObject(in.Risk) {
		return nil, ErrInvalidOperation
	}

	var result *OperationRecord
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		p, err := s.repo.GetProposalForUpdate(ctx, tx, in.ProposalID)
		if err != nil {
			return err
		}
		if p.Status != ProposalDraft {
			return fmt.Errorf("%w: status=%s", ErrProposalNotDraft, p.Status)
		}
		n, err := s.repo.CountOperations(ctx, tx, in.ProposalID)
		if err != nil {
			return err
		}
		opID, err := s.ids.New()
		if err != nil {
			return err
		}
		op := &OperationRecord{
			ID: opID, ProposalID: in.ProposalID, Sequence: n + 1,
			SchemaVersion: in.SchemaVersion, OperationType: strings.TrimSpace(in.OperationType),
			TargetPageID: in.TargetPageID, TargetBlockID: in.TargetBlockID,
			TargetNodeID: in.TargetNodeID, TargetEntityID: in.TargetEntityID,
			TargetClaimID: in.TargetClaimID, ExpectedHash: in.ExpectedHash,
			Target: compactJSON(in.Target), Base: compactJSON(in.Base), Evidence: compactJSON(in.Evidence),
			Risk: compactJSON(in.Risk), Payload: compactJSON(in.Payload),
		}
		if err := s.repo.InsertOperation(ctx, tx, op); err != nil {
			return err
		}
		ops, err := s.repo.ListOperations(ctx, tx, in.ProposalID)
		if err != nil {
			return err
		}
		result = &ops[len(ops)-1]
		return nil
	})
	return result, err
}

func (s *Service) ListOperations(ctx context.Context, proposalID uuid.UUID) ([]OperationRecord, error) {
	if _, err := s.repo.GetProposal(ctx, nil, proposalID); err != nil {
		return nil, err
	}
	return s.repo.ListOperations(ctx, nil, proposalID)
}

// Transition 执行白名单状态转换；draft 提交前必须至少有一条 Operation。
func (s *Service) Transition(ctx context.Context, proposalID uuid.UUID, to string) (*Proposal, error) {
	var result *Proposal
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		p, err := s.repo.GetProposalForUpdate(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if !proposalTransitions[p.Status][to] {
			return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, p.Status, to)
		}
		if p.Status == ProposalDraft && to == ProposalSubmitted {
			n, err := s.repo.CountOperations(ctx, tx, proposalID)
			if err != nil {
				return err
			}
			if n == 0 {
				return ErrProposalHasNoOps
			}
		}
		if err := s.repo.UpdateProposalStatus(ctx, tx, proposalID, p.Status, to); err != nil {
			return err
		}
		result, err = s.repo.GetProposal(ctx, tx, proposalID)
		return err
	})
	return result, err
}

func (s *Service) checkProposalActor(ctx context.Context, tx pgx.Tx, actorID uuid.UUID) error {
	actorType, status, err := s.repo.GetActor(ctx, tx, actorID)
	if err != nil {
		return err
	}
	if status != "active" {
		return ErrInvalidActor
	}
	if actorType == "anonymous" {
		return ErrActorNotAllowed
	}
	return nil
}

func isJSONObject(raw json.RawMessage) bool {
	var v map[string]json.RawMessage
	return json.Unmarshal(raw, &v) == nil && v != nil
}

func isJSONArray(raw json.RawMessage) bool {
	var v []json.RawMessage
	return json.Unmarshal(raw, &v) == nil && v != nil
}

func compactJSON(raw json.RawMessage) json.RawMessage {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return raw
	}
	return buf.Bytes()
}

// IsNotFound 给 API 层统一映射 404。
func IsNotFound(err error) bool { return errors.Is(err, ErrProposalNotFound) }
