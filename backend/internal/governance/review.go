package governance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

type ReviewService struct {
	repo      *Repository
	txm       *db.TxManager
	ids       *id.Generator
	evaluator *RiskEvaluator
	auth      *AuthorizationService
}

func (s *ReviewService) WithAuthorization(auth *AuthorizationService) *ReviewService {
	s.auth = auth
	return s
}

func NewReviewService(repo *Repository, txm *db.TxManager, ids *id.Generator, evaluator *RiskEvaluator) *ReviewService {
	return &ReviewService{repo: repo, txm: txm, ids: ids, evaluator: evaluator}
}

type SubmitResult struct {
	Proposal   *Proposal     `json:"proposal"`
	ReviewTask *ReviewTask   `json:"review_task"`
	Decision   *RiskDecision `json:"decision"`
}

// Submit 原子冻结 Operations、记录可解释策略结果，并自动批准低风险安全操作或入人工队列。
func (s *ReviewService) Submit(ctx context.Context, proposalID uuid.UUID) (*SubmitResult, error) {
	var result SubmitResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		p, err := s.repo.GetProposalForUpdate(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if p.Status != ProposalDraft {
			return fmt.Errorf("%w: status=%s", ErrInvalidTransition, p.Status)
		}
		ops, err := s.repo.ListOperations(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if len(ops) == 0 {
			return ErrProposalHasNoOps
		}
		decision, err := s.evaluator.Evaluate(ctx, ops)
		if err != nil {
			return err
		}
		reasons, _ := json.Marshal(decision.Reasons)
		decisionJSON, _ := json.Marshal(decision)
		if err := s.repo.UpdateProposalPolicy(ctx, tx, proposalID, decision.Level, reasons, decisionJSON); err != nil {
			return err
		}
		if err := s.repo.UpdateProposalStatus(ctx, tx, proposalID, ProposalDraft, ProposalSubmitted); err != nil {
			return err
		}
		to := ProposalInReview
		if decision.AutoApprove {
			to = ProposalApproved
		}
		if err := s.repo.UpdateProposalStatus(ctx, tx, proposalID, ProposalSubmitted, to); err != nil {
			return err
		}
		if !decision.AutoApprove {
			taskID, err := s.ids.New()
			if err != nil {
				return err
			}
			task := &ReviewTask{ID: taskID, ProposalID: proposalID, Status: ReviewPending}
			if err := s.repo.InsertReviewTask(ctx, tx, task); err != nil {
				return err
			}
			result.ReviewTask = task
		}
		result.Decision = decision
		result.Proposal, err = s.repo.GetProposal(ctx, tx, proposalID)
		return err
	})
	return &result, err
}

// Decide 仅 active human 可完成人工审核；拒绝必须提供原因。
func (s *ReviewService) Decide(ctx context.Context, taskID, reviewerID uuid.UUID, approve bool, reason string) (*Proposal, error) {
	actorType, status, err := s.repo.GetActor(ctx, nil, reviewerID)
	if err != nil || status != "active" {
		return nil, ErrInvalidActor
	}
	if actorType != "human" {
		return nil, ErrActorNotAllowed
	}
	reason = strings.TrimSpace(reason)
	if !approve && reason == "" {
		return nil, fmt.Errorf("%w: 拒绝必须说明原因", ErrInvalidProposal)
	}
	var proposal *Proposal
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		task, err := s.repo.GetReviewTaskForUpdate(ctx, tx, taskID)
		if err != nil {
			return err
		}
		if task.Status != ReviewPending {
			return ErrInvalidTransition
		}
		p, err := s.repo.GetProposalForUpdate(ctx, tx, task.ProposalID)
		if err != nil {
			return err
		}
		if p.Status != ProposalInReview {
			return ErrInvalidTransition
		}
		if s.auth != nil {
			if err := s.auth.CheckTx(ctx, tx, reviewerID, wikiIDForProposal(ctx, tx, s.repo, p), ActionReview, p.TargetID); err != nil {
				return err
			}
		}
		taskStatus, proposalStatus := ReviewRejected, ProposalRejected
		if approve {
			taskStatus, proposalStatus = ReviewApproved, ProposalApproved
		}
		if err := s.repo.DecideReviewTask(ctx, tx, task.ID, reviewerID, taskStatus, reason); err != nil {
			return err
		}
		if err := s.repo.UpdateProposalStatus(ctx, tx, p.ID, ProposalInReview, proposalStatus); err != nil {
			return err
		}
		proposal, err = s.repo.GetProposal(ctx, tx, p.ID)
		return err
	})
	return proposal, err
}

func (s *ReviewService) Pending(ctx context.Context, limit int) ([]ReviewTask, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return s.repo.ListPendingReviewTasks(ctx, limit)
}
