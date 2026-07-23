package governance

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var (
	ErrBulkReviewNotFound   = errors.New("governance: 批量审核不存在")
	ErrBulkReviewIncomplete = errors.New("governance: 抽样审核尚未完成")
	ErrBulkReviewPaused     = errors.New("governance: 批量审核已暂停")
)

const (
	BulkReviewReviewing = "reviewing"
	BulkReviewReady     = "ready"
	BulkReviewApplying  = "applying"
	BulkReviewPaused    = "paused"
	BulkReviewCompleted = "completed"

	BulkSamplingSampled = "sampled"
	BulkSamplingFull    = "full"

	BulkDecisionPending  = "pending"
	BulkDecisionApproved = "approved"
	BulkDecisionRejected = "rejected"

	BulkApplyPending = "pending"
	BulkApplyApplied = "applied"
	BulkApplyFailed  = "failed"
	BulkApplySkipped = "skipped"
)

type BulkReviewBatch struct {
	ID              uuid.UUID        `json:"id"`
	CreatedBy       uuid.UUID        `json:"created_by"`
	Status          string           `json:"status"`
	SamplingMode    string           `json:"sampling_mode"`
	SamplePercent   int              `json:"sample_percent"`
	ForceFullReason *string          `json:"force_full_reason"`
	WaveSize        int              `json:"wave_size"`
	CurrentWave     int              `json:"current_wave"`
	CreatedAt       time.Time        `json:"created_at"`
	FinalizedAt     *time.Time       `json:"finalized_at"`
	PausedAt        *time.Time       `json:"paused_at"`
	CompletedAt     *time.Time       `json:"completed_at"`
	Items           []BulkReviewItem `json:"items"`
}

type BulkReviewItem struct {
	BatchID           uuid.UUID  `json:"batch_id"`
	ProposalID        uuid.UUID  `json:"proposal_id"`
	Position          int        `json:"position"`
	Wave              int        `json:"wave"`
	SelectedForReview bool       `json:"selected_for_review"`
	Decision          string     `json:"decision"`
	DecisionReason    *string    `json:"decision_reason"`
	ReviewedBy        *uuid.UUID `json:"reviewed_by"`
	ReviewedAt        *time.Time `json:"reviewed_at"`
	ApplyStatus       string     `json:"apply_status"`
	ChangeBatchID     *uuid.UUID `json:"change_batch_id"`
	ApplyErrorCode    *string    `json:"apply_error_code"`
	AppliedAt         *time.Time `json:"applied_at"`
}

type BulkReviewAuditEvent struct {
	ID         uuid.UUID       `json:"id"`
	BatchID    uuid.UUID       `json:"batch_id"`
	ActorID    uuid.UUID       `json:"actor_id"`
	EventType  string          `json:"event_type"`
	ProposalID *uuid.UUID      `json:"proposal_id"`
	Wave       *int            `json:"wave"`
	Payload    json.RawMessage `json:"payload"`
	CreatedAt  time.Time       `json:"created_at"`
}

type CreateBulkReviewParams struct {
	ProposalIDs   []uuid.UUID
	CreatedBy     uuid.UUID
	SamplePercent int
	ForceFull     bool
	WaveSize      int
}

type BulkReviewWaveResult struct {
	BatchID uuid.UUID        `json:"batch_id"`
	Wave    int              `json:"wave"`
	Status  string           `json:"status"`
	Items   []BulkReviewItem `json:"items"`
}

type BulkReviewService struct {
	repo  *Repository
	apply *ApplyService
	txm   *db.TxManager
	ids   *id.Generator
	auth  *AuthorizationService
}

func NewBulkReviewService(repo *Repository, apply *ApplyService, txm *db.TxManager, ids *id.Generator) *BulkReviewService {
	return &BulkReviewService{repo: repo, apply: apply, txm: txm, ids: ids}
}

func (s *BulkReviewService) WithAuthorization(auth *AuthorizationService) *BulkReviewService {
	s.auth = auth
	return s
}

// Create freezes membership, deterministic sampling and wave assignment.
func (s *BulkReviewService) Create(ctx context.Context, in CreateBulkReviewParams) (*BulkReviewBatch, error) {
	if in.CreatedBy == uuid.Nil || len(in.ProposalIDs) == 0 || len(in.ProposalIDs) > 1000 {
		return nil, ErrInvalidProposal
	}
	if in.SamplePercent <= 0 || in.SamplePercent > 100 || in.WaveSize <= 0 || in.WaveSize > 1000 {
		return nil, ErrInvalidProposal
	}
	if err := s.requireHuman(ctx, in.CreatedBy); err != nil {
		return nil, err
	}
	seen := make(map[uuid.UUID]struct{}, len(in.ProposalIDs))
	for _, proposalID := range in.ProposalIDs {
		if proposalID == uuid.Nil {
			return nil, ErrInvalidProposal
		}
		if _, exists := seen[proposalID]; exists {
			return nil, fmt.Errorf("%w: Proposal 重复", ErrInvalidProposal)
		}
		seen[proposalID] = struct{}{}
	}

	batchID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	var result *BulkReviewBatch
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		full := in.ForceFull
		reason := ""
		for _, proposalID := range in.ProposalIDs {
			proposal, err := s.repo.GetProposalForUpdate(ctx, tx, proposalID)
			if err != nil {
				return err
			}
			if proposal.Status != ProposalInReview {
				return fmt.Errorf("%w: proposal=%s status=%s", ErrInvalidTransition, proposalID, proposal.Status)
			}
			if err := s.checkReviewAuthorization(ctx, tx, in.CreatedBy, proposal); err != nil {
				return err
			}
			if proposal.RiskLevel == RiskHigh || proposal.RiskLevel == RiskCritical {
				full = true
				reason = "high_or_critical_risk"
			}
			if _, err := s.pendingTaskID(ctx, tx, proposalID); err != nil {
				return err
			}
		}
		if in.ForceFull {
			reason = "forced_by_reviewer"
		}
		mode := BulkSamplingSampled
		var reasonPtr *string
		if full {
			mode = BulkSamplingFull
			reasonPtr = &reason
		}
		if _, err := tx.Exec(ctx, `INSERT INTO bulk_review_batch
			(id,created_by,status,sampling_mode,sample_percent,force_full_reason,wave_size)
			VALUES ($1,$2,$3,$4,$5,$6,$7)`,
			batchID, in.CreatedBy, BulkReviewReviewing, mode, in.SamplePercent, reasonPtr, in.WaveSize); err != nil {
			return err
		}
		selected := sampledProposalIDs(batchID, in.ProposalIDs, in.SamplePercent, full)
		for index, proposalID := range in.ProposalIDs {
			_, err := tx.Exec(ctx, `INSERT INTO bulk_review_batch_item
				(batch_id,proposal_id,position,wave,selected_for_review)
				VALUES ($1,$2,$3,$4,$5)`,
				batchID, proposalID, index+1, index/in.WaveSize+1, selected[proposalID])
			if err != nil {
				return err
			}
		}
		if err := s.audit(ctx, tx, batchID, in.CreatedBy, "bulk_review.created", nil, nil, map[string]any{
			"sampling_mode": mode, "sample_percent": in.SamplePercent, "wave_size": in.WaveSize,
			"proposal_count": len(in.ProposalIDs),
		}); err != nil {
			return err
		}
		result, err = s.getTx(ctx, tx, batchID, false)
		return err
	})
	return result, err
}

func sampledProposalIDs(batchID uuid.UUID, proposalIDs []uuid.UUID, percent int, full bool) map[uuid.UUID]bool {
	selected := make(map[uuid.UUID]bool, len(proposalIDs))
	if full {
		for _, proposalID := range proposalIDs {
			selected[proposalID] = true
		}
		return selected
	}
	type scored struct {
		id    uuid.UUID
		score [32]byte
	}
	scores := make([]scored, 0, len(proposalIDs))
	for _, proposalID := range proposalIDs {
		scores = append(scores, scored{id: proposalID, score: sha256.Sum256(append(batchID[:], proposalID[:]...))})
	}
	sort.Slice(scores, func(i, j int) bool {
		return string(scores[i].score[:]) < string(scores[j].score[:])
	})
	count := (len(scores)*percent + 99) / 100
	if count < 1 {
		count = 1
	}
	for _, candidate := range scores[:count] {
		selected[candidate.id] = true
	}
	return selected
}

func (s *BulkReviewService) Get(ctx context.Context, batchID, actorID uuid.UUID) (*BulkReviewBatch, error) {
	var result *BulkReviewBatch
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.getTx(ctx, tx, batchID, false)
		if err != nil {
			return err
		}
		if err := s.checkBatchAuthorization(ctx, tx, actorID, batch, ActionReview); err != nil {
			return err
		}
		result = batch
		return nil
	})
	return result, err
}

// Decide records an explicit Proposal-granular decision. Sampled items must all be decided before Finalize.
func (s *BulkReviewService) Decide(ctx context.Context, batchID, proposalID, reviewerID uuid.UUID, approve bool, reason string) (*BulkReviewBatch, error) {
	if err := s.requireHuman(ctx, reviewerID); err != nil {
		return nil, err
	}
	reason = strings.TrimSpace(reason)
	if !approve && reason == "" {
		return nil, fmt.Errorf("%w: 拒绝必须说明原因", ErrInvalidProposal)
	}
	var result *BulkReviewBatch
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.getTx(ctx, tx, batchID, true)
		if err != nil {
			return err
		}
		if batch.Status != BulkReviewReviewing {
			return ErrInvalidTransition
		}
		item := findBulkItem(batch.Items, proposalID)
		if item == nil || item.Decision != BulkDecisionPending {
			return ErrInvalidTransition
		}
		proposal, err := s.repo.GetProposalForUpdate(ctx, tx, proposalID)
		if err != nil {
			return err
		}
		if err := s.checkReviewAuthorization(ctx, tx, reviewerID, proposal); err != nil {
			return err
		}
		decision := BulkDecisionRejected
		if approve {
			decision = BulkDecisionApproved
		}
		if err := s.decideProposalTx(ctx, tx, proposalID, reviewerID, decision, reason); err != nil {
			return err
		}
		applyStatus := BulkApplyPending
		if !approve {
			applyStatus = BulkApplySkipped
		}
		if _, err := tx.Exec(ctx, `UPDATE bulk_review_batch_item SET decision=$3,decision_reason=$4,
			reviewed_by=$5,reviewed_at=now(),apply_status=$6 WHERE batch_id=$1 AND proposal_id=$2`,
			batchID, proposalID, decision, nullableString(reason), reviewerID, applyStatus); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, batchID, reviewerID, "bulk_review.proposal_decided", &proposalID, &item.Wave,
			map[string]any{"decision": decision, "reason": reason}); err != nil {
			return err
		}
		result, err = s.getTx(ctx, tx, batchID, false)
		return err
	})
	return result, err
}

// Finalize approves unsampled proposals only after every sampled proposal has an explicit decision.
func (s *BulkReviewService) Finalize(ctx context.Context, batchID, reviewerID uuid.UUID) (*BulkReviewBatch, error) {
	if err := s.requireHuman(ctx, reviewerID); err != nil {
		return nil, err
	}
	var result *BulkReviewBatch
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.getTx(ctx, tx, batchID, true)
		if err != nil {
			return err
		}
		if batch.Status != BulkReviewReviewing {
			return ErrInvalidTransition
		}
		for i := range batch.Items {
			if batch.Items[i].SelectedForReview && batch.Items[i].Decision == BulkDecisionPending {
				return ErrBulkReviewIncomplete
			}
			proposal, err := s.repo.GetProposalForUpdate(ctx, tx, batch.Items[i].ProposalID)
			if err != nil {
				return err
			}
			if err := s.checkReviewAuthorization(ctx, tx, reviewerID, proposal); err != nil {
				return err
			}
		}
		approved := 0
		for i := range batch.Items {
			item := &batch.Items[i]
			if item.Decision == BulkDecisionPending {
				if err := s.decideProposalTx(ctx, tx, item.ProposalID, reviewerID, BulkDecisionApproved, "risk_sample_passed"); err != nil {
					return err
				}
				if _, err := tx.Exec(ctx, `UPDATE bulk_review_batch_item SET decision='approved',
					decision_reason='risk_sample_passed',reviewed_by=$3,reviewed_at=now()
					WHERE batch_id=$1 AND proposal_id=$2`, batchID, item.ProposalID, reviewerID); err != nil {
					return err
				}
			}
			if item.Decision != BulkDecisionRejected {
				approved++
			}
		}
		status := BulkReviewReady
		if approved == 0 {
			status = BulkReviewCompleted
		}
		if _, err := tx.Exec(ctx, `UPDATE bulk_review_batch SET status=$2,finalized_at=now(),
			completed_at=CASE WHEN $2='completed' THEN now() ELSE NULL END WHERE id=$1`,
			batchID, status); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, batchID, reviewerID, "bulk_review.finalized", nil, nil,
			map[string]any{"approved_count": approved, "status": status}); err != nil {
			return err
		}
		result, err = s.getTx(ctx, tx, batchID, false)
		return err
	})
	return result, err
}

func (s *BulkReviewService) Pause(ctx context.Context, batchID, actorID uuid.UUID) (*BulkReviewBatch, error) {
	return s.setPaused(ctx, batchID, actorID, true)
}

func (s *BulkReviewService) Resume(ctx context.Context, batchID, actorID uuid.UUID) (*BulkReviewBatch, error) {
	return s.setPaused(ctx, batchID, actorID, false)
}

func (s *BulkReviewService) setPaused(ctx context.Context, batchID, actorID uuid.UUID, pause bool) (*BulkReviewBatch, error) {
	if err := s.requireHuman(ctx, actorID); err != nil {
		return nil, err
	}
	var result *BulkReviewBatch
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.getTx(ctx, tx, batchID, true)
		if err != nil {
			return err
		}
		if err := s.checkBatchAuthorization(ctx, tx, actorID, batch, ActionApply); err != nil {
			return err
		}
		from, to, eventType := batch.Status, BulkReviewPaused, "bulk_review.paused"
		if pause {
			if from != BulkReviewReady && from != BulkReviewApplying {
				return ErrInvalidTransition
			}
		} else {
			if from != BulkReviewPaused {
				return ErrInvalidTransition
			}
			to, eventType = BulkReviewReady, "bulk_review.resumed"
		}
		if _, err := tx.Exec(ctx, `UPDATE bulk_review_batch SET status=$2,
			paused_at=CASE WHEN $2='paused' THEN now() ELSE NULL END WHERE id=$1`, batchID, to); err != nil {
			return err
		}
		if err := s.audit(ctx, tx, batchID, actorID, eventType, nil, nil, map[string]any{"from": from, "to": to}); err != nil {
			return err
		}
		result, err = s.getTx(ctx, tx, batchID, false)
		return err
	})
	return result, err
}

// ApplyNextWave invokes the existing single-Proposal Apply boundary for one immutable wave.
func (s *BulkReviewService) ApplyNextWave(ctx context.Context, batchID, actorID uuid.UUID) (*BulkReviewWaveResult, error) {
	if s.apply == nil {
		return nil, errors.New("governance: 批量审核 Apply 服务未配置")
	}
	var wave int
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.getTx(ctx, tx, batchID, true)
		if err != nil {
			return err
		}
		if err := s.checkBatchAuthorization(ctx, tx, actorID, batch, ActionApply); err != nil {
			return err
		}
		if batch.Status == BulkReviewPaused {
			return ErrBulkReviewPaused
		}
		if batch.Status != BulkReviewReady {
			return ErrInvalidTransition
		}
		err = tx.QueryRow(ctx, `SELECT COALESCE(min(wave),0) FROM bulk_review_batch_item
			WHERE batch_id=$1 AND decision='approved' AND apply_status IN ('pending','failed')`, batchID).Scan(&wave)
		if err != nil {
			return err
		}
		if wave == 0 {
			_, err = tx.Exec(ctx, `UPDATE bulk_review_batch SET status='completed',completed_at=now() WHERE id=$1`, batchID)
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE bulk_review_batch SET status='applying',current_wave=$2 WHERE id=$1`, batchID, wave); err != nil {
			return err
		}
		return s.audit(ctx, tx, batchID, actorID, "bulk_review.wave_started", nil, &wave, map[string]any{"wave": wave})
	})
	if err != nil {
		return nil, err
	}

	batch, err := s.getTx(ctx, nil, batchID, false)
	if err != nil {
		return nil, err
	}
	var waveItems []BulkReviewItem
	failed := false
	for i := range batch.Items {
		item := batch.Items[i]
		if item.Wave != wave || item.Decision != BulkDecisionApproved ||
			(item.ApplyStatus != BulkApplyPending && item.ApplyStatus != BulkApplyFailed) {
			continue
		}
		current, getErr := s.getTx(ctx, nil, batchID, false)
		if getErr != nil {
			return nil, getErr
		}
		if current.Status == BulkReviewPaused {
			return &BulkReviewWaveResult{BatchID: batchID, Wave: wave, Status: current.Status, Items: waveItems}, nil
		}
		applied, applyErr := s.apply.Apply(ctx, item.ProposalID, actorID)
		if applyErr != nil {
			failed = true
			if err := s.recordApply(ctx, batchID, actorID, item, nil, "apply_failed"); err != nil {
				return nil, err
			}
		} else {
			if err := s.recordApply(ctx, batchID, actorID, item, &applied.ChangeBatchID, ""); err != nil {
				return nil, err
			}
		}
	}

	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.getTx(ctx, tx, batchID, true)
		if err != nil {
			return err
		}
		if batch.Status == BulkReviewPaused {
			return s.audit(ctx, tx, batchID, actorID, "bulk_review.wave_finished", nil, &wave,
				map[string]any{"wave": wave, "status": BulkReviewPaused})
		}
		if batch.Status != BulkReviewApplying {
			return ErrInvalidTransition
		}
		status := BulkReviewReady
		if failed {
			status = BulkReviewPaused
		} else {
			var remaining int
			if err := tx.QueryRow(ctx, `SELECT count(*) FROM bulk_review_batch_item
				WHERE batch_id=$1 AND decision='approved' AND apply_status IN ('pending','failed')`, batchID).Scan(&remaining); err != nil {
				return err
			}
			if remaining == 0 {
				status = BulkReviewCompleted
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE bulk_review_batch SET status=$2,
			paused_at=CASE WHEN $2='paused' THEN now() ELSE paused_at END,
			completed_at=CASE WHEN $2='completed' THEN now() ELSE completed_at END
			WHERE id=$1`, batchID, status); err != nil {
			return err
		}
		return s.audit(ctx, tx, batchID, actorID, "bulk_review.wave_finished", nil, &wave,
			map[string]any{"wave": wave, "status": status})
	})
	if err != nil {
		return nil, err
	}
	final, err := s.getTx(ctx, nil, batchID, false)
	if err != nil {
		return nil, err
	}
	for _, item := range final.Items {
		if item.Wave == wave {
			waveItems = append(waveItems, item)
		}
	}
	return &BulkReviewWaveResult{BatchID: batchID, Wave: wave, Status: final.Status, Items: waveItems}, nil
}

func (s *BulkReviewService) recordApply(ctx context.Context, batchID, actorID uuid.UUID, item BulkReviewItem, changeBatchID *uuid.UUID, code string) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		status, eventType := BulkApplyApplied, "bulk_review.proposal_applied"
		var errorCode *string
		if changeBatchID == nil {
			status, eventType, errorCode = BulkApplyFailed, "bulk_review.proposal_apply_failed", &code
		}
		if _, err := tx.Exec(ctx, `UPDATE bulk_review_batch_item SET apply_status=$3,
			change_batch_id=$4,apply_error_code=$5,applied_at=CASE WHEN $3='applied' THEN now() ELSE NULL END
			WHERE batch_id=$1 AND proposal_id=$2`, batchID, item.ProposalID, status, changeBatchID, errorCode); err != nil {
			return err
		}
		return s.audit(ctx, tx, batchID, actorID, eventType, &item.ProposalID, &item.Wave,
			map[string]any{"apply_status": status, "change_batch_id": changeBatchID, "error_code": errorCode})
	})
}

func (s *BulkReviewService) Audit(ctx context.Context, batchID, actorID uuid.UUID) ([]BulkReviewAuditEvent, error) {
	var events []BulkReviewAuditEvent
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		batch, err := s.getTx(ctx, tx, batchID, false)
		if err != nil {
			return err
		}
		if err := s.checkBatchAuthorization(ctx, tx, actorID, batch, ActionReview); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT id,batch_id,actor_id,event_type,proposal_id,wave,payload_json,created_at
			FROM bulk_review_audit_event WHERE batch_id=$1 ORDER BY created_at,id`, batchID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var event BulkReviewAuditEvent
			if err := rows.Scan(&event.ID, &event.BatchID, &event.ActorID, &event.EventType,
				&event.ProposalID, &event.Wave, &event.Payload, &event.CreatedAt); err != nil {
				return err
			}
			events = append(events, event)
		}
		return rows.Err()
	})
	return events, err
}

func (s *BulkReviewService) getTx(ctx context.Context, tx pgx.Tx, batchID uuid.UUID, forUpdate bool) (*BulkReviewBatch, error) {
	query := `SELECT id,created_by,status,sampling_mode,sample_percent,force_full_reason,wave_size,
		current_wave,created_at,finalized_at,paused_at,completed_at FROM bulk_review_batch WHERE id=$1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	var batch BulkReviewBatch
	err := s.repo.q(tx).QueryRow(ctx, query, batchID).Scan(&batch.ID, &batch.CreatedBy, &batch.Status,
		&batch.SamplingMode, &batch.SamplePercent, &batch.ForceFullReason, &batch.WaveSize,
		&batch.CurrentWave, &batch.CreatedAt, &batch.FinalizedAt, &batch.PausedAt, &batch.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBulkReviewNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.repo.q(tx).Query(ctx, `SELECT batch_id,proposal_id,position,wave,selected_for_review,
		decision,decision_reason,reviewed_by,reviewed_at,apply_status,change_batch_id,apply_error_code,applied_at
		FROM bulk_review_batch_item WHERE batch_id=$1 ORDER BY position`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	batch.Items = []BulkReviewItem{}
	for rows.Next() {
		var item BulkReviewItem
		if err := rows.Scan(&item.BatchID, &item.ProposalID, &item.Position, &item.Wave,
			&item.SelectedForReview, &item.Decision, &item.DecisionReason, &item.ReviewedBy,
			&item.ReviewedAt, &item.ApplyStatus, &item.ChangeBatchID, &item.ApplyErrorCode,
			&item.AppliedAt); err != nil {
			return nil, err
		}
		batch.Items = append(batch.Items, item)
	}
	return &batch, rows.Err()
}

func (s *BulkReviewService) decideProposalTx(ctx context.Context, tx pgx.Tx, proposalID, reviewerID uuid.UUID, decision, reason string) error {
	taskID, err := s.pendingTaskID(ctx, tx, proposalID)
	if err != nil {
		return err
	}
	reviewStatus, proposalStatus := ReviewApproved, ProposalApproved
	if decision == BulkDecisionRejected {
		reviewStatus, proposalStatus = ReviewRejected, ProposalRejected
	}
	if err := s.repo.DecideReviewTask(ctx, tx, taskID, reviewerID, reviewStatus, reason); err != nil {
		return err
	}
	return s.repo.UpdateProposalStatus(ctx, tx, proposalID, ProposalInReview, proposalStatus)
}

func (s *BulkReviewService) pendingTaskID(ctx context.Context, tx pgx.Tx, proposalID uuid.UUID) (uuid.UUID, error) {
	var taskID uuid.UUID
	err := s.repo.q(tx).QueryRow(ctx, `SELECT id FROM review_task
		WHERE proposal_id=$1 AND status='pending' FOR UPDATE`, proposalID).Scan(&taskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrApprovalRequired
	}
	return taskID, err
}

func (s *BulkReviewService) requireHuman(ctx context.Context, actorID uuid.UUID) error {
	actorType, status, err := s.repo.GetActor(ctx, nil, actorID)
	if err != nil || status != "active" {
		return ErrInvalidActor
	}
	if actorType != "human" && actorType != "system" {
		return ErrActorNotAllowed
	}
	return nil
}

func (s *BulkReviewService) checkReviewAuthorization(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, proposal *Proposal) error {
	return s.checkProposalAuthorization(ctx, tx, actorID, proposal, ActionReview)
}

func (s *BulkReviewService) checkBatchAuthorization(ctx context.Context, tx pgx.Tx, actorID uuid.UUID,
	batch *BulkReviewBatch, action string) error {
	if s.auth == nil {
		return nil
	}
	for i := range batch.Items {
		proposal, err := s.repo.GetProposal(ctx, tx, batch.Items[i].ProposalID)
		if err != nil {
			return err
		}
		if err := s.checkProposalAuthorization(ctx, tx, actorID, proposal, action); err != nil {
			return err
		}
	}
	return nil
}

func (s *BulkReviewService) checkProposalAuthorization(ctx context.Context, tx pgx.Tx, actorID uuid.UUID,
	proposal *Proposal, action string) error {
	if s.auth == nil {
		return nil
	}
	return s.auth.CheckTx(ctx, tx, actorID, wikiIDForProposal(ctx, tx, s.repo, proposal), action, proposal.TargetID)
}

func (s *BulkReviewService) audit(ctx context.Context, tx pgx.Tx, batchID, actorID uuid.UUID,
	eventType string, proposalID *uuid.UUID, wave *int, payload any) error {
	eventID, err := s.ids.New()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO bulk_review_audit_event
		(id,batch_id,actor_id,event_type,proposal_id,wave,payload_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`,
		eventID, batchID, actorID, eventType, proposalID, wave, raw)
	return err
}

func findBulkItem(items []BulkReviewItem, proposalID uuid.UUID) *BulkReviewItem {
	for i := range items {
		if items[i].ProposalID == proposalID {
			return &items[i]
		}
	}
	return nil
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}
