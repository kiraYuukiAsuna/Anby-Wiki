package governance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
)

type Repository struct{ pool db.Querier }

func NewRepository(pool db.Querier) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(tx pgx.Tx) db.Querier {
	if tx != nil {
		return tx
	}
	return r.pool
}

const proposalColumns = `id, import_job_id, target_type, target_id, base_revision_id,
	base_state_version, status, risk_level, risk_reasons_json, policy_decision_json,
	created_by, idempotency_key,
	(SELECT cb.id FROM change_batch cb WHERE cb.proposal_id=proposal.id LIMIT 1),
	(SELECT cb.status FROM change_batch cb WHERE cb.proposal_id=proposal.id LIMIT 1),
	created_at, updated_at`

func scanProposal(row pgx.Row) (*Proposal, error) {
	var p Proposal
	if err := row.Scan(&p.ID, &p.ImportJobID, &p.TargetType, &p.TargetID, &p.BaseRevisionID,
		&p.BaseStateVersion, &p.Status, &p.RiskLevel, &p.RiskReasons, &p.PolicyDecision,
		&p.CreatedBy, &p.IdempotencyKey, &p.ChangeBatchID, &p.ChangeBatchStatus,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *Repository) GetProposal(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Proposal, error) {
	p, err := scanProposal(r.q(tx).QueryRow(ctx, `SELECT `+proposalColumns+` FROM proposal WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrProposalNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("governance: 查询 Proposal 失败: %w", err)
	}
	return p, nil
}

func (r *Repository) GetProposalForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*Proposal, error) {
	p, err := scanProposal(r.q(tx).QueryRow(ctx, `SELECT `+proposalColumns+` FROM proposal WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrProposalNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("governance: 锁定 Proposal 失败: %w", err)
	}
	return p, nil
}

func (r *Repository) GetProposalByIdempotency(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, key string) (*Proposal, error) {
	p, err := scanProposal(r.q(tx).QueryRow(ctx, `SELECT `+proposalColumns+`
		FROM proposal WHERE created_by=$1 AND idempotency_key=$2`, actorID, key))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrProposalNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("governance: 按幂等键查询 Proposal 失败: %w", err)
	}
	return p, nil
}

type proposalListCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

func encodeProposalCursor(value proposalListCursor) string {
	raw, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func decodeProposalCursor(cursor string) (proposalListCursor, error) {
	var value proposalListCursor
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return value, err
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return value, err
	}
	return value, nil
}

func (r *Repository) ListOwnedProposals(
	ctx context.Context, actorID uuid.UUID, status, targetType, cursor string, limit int,
) (*ProposalPage, error) {
	var afterTime *time.Time
	afterID := uuid.Nil
	if cursor != "" {
		after, err := decodeProposalCursor(cursor)
		if err != nil || after.CreatedAt.IsZero() || after.ID == uuid.Nil {
			return nil, ErrInvalidProposalCursor
		}
		afterTime = &after.CreatedAt
		afterID = after.ID
	}
	rows, err := r.pool.Query(ctx, `SELECT `+proposalColumns+` FROM proposal
		WHERE created_by=$1
		  AND ($2='' OR status=$2)
		  AND ($3='' OR target_type=$3)
		  AND ($4::timestamptz IS NULL OR (proposal.created_at,proposal.id) < ($4,$5))
		ORDER BY proposal.created_at DESC,proposal.id DESC
		LIMIT $6`, actorID, status, targetType, afterTime, afterID, limit+1)
	if err != nil {
		return nil, fmt.Errorf("governance: 列出 Proposal 失败: %w", err)
	}
	defer rows.Close()
	result := &ProposalPage{Items: make([]Proposal, 0, limit)}
	for rows.Next() {
		proposal, err := scanProposal(rows)
		if err != nil {
			return nil, fmt.Errorf("governance: 扫描 Proposal 失败: %w", err)
		}
		result.Items = append(result.Items, *proposal)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > limit {
		last := result.Items[limit-1]
		next := encodeProposalCursor(proposalListCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		result.NextCursor = &next
		result.Items = result.Items[:limit]
	}
	return result, nil
}

func (r *Repository) InsertProposal(ctx context.Context, tx pgx.Tx, p *Proposal) (bool, error) {
	tag, err := r.q(tx).Exec(ctx, `
		INSERT INTO proposal (id, import_job_id, target_type, target_id, base_revision_id,
			base_state_version, status, risk_level, created_by, idempotency_key)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (created_by, idempotency_key) DO NOTHING`,
		p.ID, p.ImportJobID, p.TargetType, p.TargetID, p.BaseRevisionID, p.BaseStateVersion,
		p.Status, p.RiskLevel, p.CreatedBy, p.IdempotencyKey)
	if err != nil {
		return false, fmt.Errorf("governance: 插入 Proposal 失败: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

func (r *Repository) UpdateProposalStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, from, to string) error {
	tag, err := r.q(tx).Exec(ctx, `UPDATE proposal SET status=$3, updated_at=now() WHERE id=$1 AND status=$2`, id, from, to)
	if err != nil {
		return fmt.Errorf("governance: 更新 Proposal 状态失败: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: %s -> %s", ErrInvalidTransition, from, to)
	}
	return nil
}

func (r *Repository) UpdateProposalPolicy(ctx context.Context, tx pgx.Tx, id uuid.UUID, level string, reasons, decision json.RawMessage) error {
	_, err := r.q(tx).Exec(ctx, `UPDATE proposal SET risk_level=$2, risk_reasons_json=$3,
		policy_decision_json=$4, updated_at=now() WHERE id=$1`, id, level, reasons, decision)
	if err != nil {
		return fmt.Errorf("governance: 更新风险策略结果失败: %w", err)
	}
	return nil
}

func (r *Repository) CountOperations(ctx context.Context, tx pgx.Tx, proposalID uuid.UUID) (int, error) {
	var n int
	if err := r.q(tx).QueryRow(ctx, `SELECT count(*) FROM proposal_operation WHERE proposal_id=$1`, proposalID).Scan(&n); err != nil {
		return 0, fmt.Errorf("governance: 统计 Operation 失败: %w", err)
	}
	return n, nil
}

func (r *Repository) InsertOperation(ctx context.Context, tx pgx.Tx, op *OperationRecord) error {
	_, err := r.q(tx).Exec(ctx, `
		INSERT INTO proposal_operation (id, proposal_id, sequence, schema_version, operation_type,
			target_page_id, target_block_id, target_node_id, target_entity_id, target_claim_id,
			target_json, expected_hash, base_json, evidence_json, risk_json, payload_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)`,
		op.ID, op.ProposalID, op.Sequence, op.SchemaVersion, op.OperationType,
		op.TargetPageID, op.TargetBlockID, op.TargetNodeID, op.TargetEntityID, op.TargetClaimID,
		op.Target, op.ExpectedHash, op.Base, op.Evidence, op.Risk, op.Payload)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrOperationSequenceRace, err)
	}
	return nil
}

func (r *Repository) ListOperations(ctx context.Context, tx pgx.Tx, proposalID uuid.UUID) ([]OperationRecord, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT id, proposal_id, sequence, schema_version, operation_type,
			target_page_id, target_block_id, target_node_id, target_entity_id, target_claim_id,
			target_json, expected_hash, base_json, evidence_json, risk_json, payload_json, created_at
		FROM proposal_operation WHERE proposal_id=$1 ORDER BY sequence`, proposalID)
	if err != nil {
		return nil, fmt.Errorf("governance: 列出 Operation 失败: %w", err)
	}
	defer rows.Close()
	var out []OperationRecord
	for rows.Next() {
		var op OperationRecord
		if err := rows.Scan(&op.ID, &op.ProposalID, &op.Sequence, &op.SchemaVersion, &op.OperationType,
			&op.TargetPageID, &op.TargetBlockID, &op.TargetNodeID, &op.TargetEntityID, &op.TargetClaimID,
			&op.Target, &op.ExpectedHash, &op.Base, &op.Evidence, &op.Risk, &op.Payload, &op.CreatedAt); err != nil {
			return nil, fmt.Errorf("governance: 扫描 Operation 失败: %w", err)
		}
		out = append(out, op)
	}
	return out, rows.Err()
}

func (r *Repository) GetActor(ctx context.Context, tx pgx.Tx, actorID uuid.UUID) (actorType, status string, err error) {
	err = r.q(tx).QueryRow(ctx, `SELECT actor_type, status FROM actor WHERE id=$1`, actorID).Scan(&actorType, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrInvalidActor
	}
	if err != nil {
		return "", "", fmt.Errorf("governance: 查询 Actor 失败: %w", err)
	}
	return actorType, status, nil
}

func (r *Repository) InsertReviewTask(ctx context.Context, tx pgx.Tx, task *ReviewTask) error {
	return r.q(tx).QueryRow(ctx, `
		INSERT INTO review_task (id,proposal_id,status) VALUES ($1,$2,'pending')
		RETURNING created_at`, task.ID, task.ProposalID).Scan(&task.CreatedAt)
}

func scanReviewTask(row pgx.Row) (*ReviewTask, error) {
	var task ReviewTask
	if err := row.Scan(&task.ID, &task.ProposalID, &task.Status, &task.ReviewerID,
		&task.DecisionReason, &task.CreatedAt, &task.ReviewedAt); err != nil {
		return nil, err
	}
	return &task, nil
}

const reviewTaskColumns = `id, proposal_id, status, reviewer_id, decision_reason, created_at, reviewed_at`

func (r *Repository) GetReviewTaskForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*ReviewTask, error) {
	task, err := scanReviewTask(r.q(tx).QueryRow(ctx, `SELECT `+reviewTaskColumns+` FROM review_task WHERE id=$1 FOR UPDATE`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: review_task=%s", ErrProposalNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("governance: 锁定 ReviewTask 失败: %w", err)
	}
	return task, nil
}

func (r *Repository) DecideReviewTask(ctx context.Context, tx pgx.Tx, id, reviewer uuid.UUID, status, reason string) error {
	tag, err := r.q(tx).Exec(ctx, `UPDATE review_task SET status=$2, reviewer_id=$3,
		decision_reason=$4, reviewed_at=now() WHERE id=$1 AND status='pending'`, id, status, reviewer, reason)
	if err != nil {
		return fmt.Errorf("governance: 更新 ReviewTask 失败: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) ListPendingReviewTasks(ctx context.Context, limit int) ([]ReviewTask, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+reviewTaskColumns+` FROM review_task
		WHERE status='pending' ORDER BY created_at,id LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("governance: 查询审核队列失败: %w", err)
	}
	defer rows.Close()
	out := make([]ReviewTask, 0)
	for rows.Next() {
		task, err := scanReviewTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *task)
	}
	return out, rows.Err()
}

func (r *Repository) InsertMergeConflict(ctx context.Context, tx pgx.Tx, c *MergeConflict) error {
	return r.q(tx).QueryRow(ctx, `
		INSERT INTO merge_conflict (id,proposal_id,page_id,conflict_type,target_block_id,target_claim_id,
			base_revision_id,current_revision_id,base_value_json,current_value_json,proposed_value_json,status)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,'open') RETURNING created_at`,
		c.ID, c.ProposalID, c.PageID, c.ConflictType, c.TargetBlockID, c.TargetClaimID,
		c.BaseRevisionID, c.CurrentRevisionID, nullableJSON(c.BaseValue),
		nullableJSON(c.CurrentValue), nullableJSON(c.ProposedValue)).Scan(&c.CreatedAt)
}

func nullableJSON(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return raw
}

func (r *Repository) ListMergeConflicts(ctx context.Context, proposalID uuid.UUID) ([]MergeConflict, error) {
	rows, err := r.pool.Query(ctx, `SELECT id,proposal_id,page_id,conflict_type,target_block_id,target_claim_id,
		base_revision_id,current_revision_id,base_value_json,current_value_json,proposed_value_json,
		status,resolved_by,resolution_json,resolved_at,created_at
		FROM merge_conflict WHERE proposal_id=$1 ORDER BY created_at,id`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MergeConflict
	for rows.Next() {
		var c MergeConflict
		if err := rows.Scan(&c.ID, &c.ProposalID, &c.PageID, &c.ConflictType,
			&c.TargetBlockID, &c.TargetClaimID, &c.BaseRevisionID, &c.CurrentRevisionID,
			&c.BaseValue, &c.CurrentValue, &c.ProposedValue, &c.Status, &c.ResolvedBy,
			&c.Resolution, &c.ResolvedAt, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *Repository) ResolveMergeConflict(
	ctx context.Context,
	tx pgx.Tx,
	proposalID, conflictID, actorID uuid.UUID,
	status string,
	resolution json.RawMessage,
) error {
	tag, err := r.q(tx).Exec(ctx, `UPDATE merge_conflict
		SET status=$4,resolved_by=$3,resolution_json=$5,resolved_at=now()
		WHERE id=$1 AND proposal_id=$2 AND status='open'`,
		conflictID, proposalID, actorID, status, resolution)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) CountOpenMergeConflicts(
	ctx context.Context,
	tx pgx.Tx,
	proposalID uuid.UUID,
) (int, error) {
	var count int
	err := r.q(tx).QueryRow(ctx, `SELECT count(*) FROM merge_conflict
		WHERE proposal_id=$1 AND status='open'`, proposalID).Scan(&count)
	return count, err
}

func (r *Repository) GetChangeBatchByProposal(ctx context.Context, tx pgx.Tx, proposalID uuid.UUID) (*ChangeBatch, error) {
	var b ChangeBatch
	err := r.q(tx).QueryRow(ctx, `SELECT id,import_job_id,proposal_id,actor_id,status,created_at,rolled_back_at
		FROM change_batch WHERE proposal_id=$1`, proposalID).Scan(&b.ID, &b.ImportJobID, &b.ProposalID,
		&b.ActorID, &b.Status, &b.CreatedAt, &b.RolledBackAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("governance: 查询 ChangeBatch 失败: %w", err)
	}
	return &b, nil
}

func (r *Repository) InsertChangeBatch(ctx context.Context, tx pgx.Tx, b *ChangeBatch) error {
	return r.q(tx).QueryRow(ctx, `INSERT INTO change_batch
		(id,import_job_id,proposal_id,actor_id,status) VALUES ($1,$2,$3,$4,$5) RETURNING created_at`,
		b.ID, b.ImportJobID, b.ProposalID, b.ActorID, b.Status).Scan(&b.CreatedAt)
}

func (r *Repository) UpdateChangeBatchStatus(ctx context.Context, tx pgx.Tx, id uuid.UUID, from, to string) error {
	tag, err := r.q(tx).Exec(ctx, `UPDATE change_batch SET status=$3,
		rolled_back_at=CASE WHEN $3='rolled_back' THEN now() ELSE rolled_back_at END
		WHERE id=$1 AND status=$2`, id, from, to)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidTransition
	}
	return nil
}

func (r *Repository) InsertAudit(ctx context.Context, tx pgx.Tx, id, actorID uuid.UUID,
	eventType, aggregateType string, aggregateID, batchID uuid.UUID, payload json.RawMessage) error {
	_, err := r.q(tx).Exec(ctx, `INSERT INTO audit_event
		(id,actor_id,event_type,aggregate_type,aggregate_id,change_batch_id,payload_json)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, id, actorID, eventType, aggregateType, aggregateID, batchID, payload)
	return err
}

func (r *Repository) InsertAuditWithoutBatch(
	ctx context.Context,
	tx pgx.Tx,
	id, actorID uuid.UUID,
	eventType, aggregateType string,
	aggregateID uuid.UUID,
	payload json.RawMessage,
) error {
	_, err := r.q(tx).Exec(ctx, `INSERT INTO audit_event
		(id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		id, actorID, eventType, aggregateType, aggregateID, payload)
	return err
}

func (r *Repository) InsertOutbox(ctx context.Context, tx pgx.Tx, id uuid.UUID,
	aggregateType string, aggregateID uuid.UUID, eventType string, payload json.RawMessage) error {
	_, err := r.q(tx).Exec(ctx, `INSERT INTO outbox_event
		(id,aggregate_type,aggregate_id,event_type,payload_json) VALUES ($1,$2,$3,$4,$5)`,
		id, aggregateType, aggregateID, eventType, payload)
	return err
}

func (r *Repository) GetChangeBatchForUpdate(ctx context.Context, tx pgx.Tx, id uuid.UUID) (*ChangeBatch, error) {
	var b ChangeBatch
	err := r.q(tx).QueryRow(ctx, `SELECT id,import_job_id,proposal_id,actor_id,status,created_at,rolled_back_at
		FROM change_batch WHERE id=$1 FOR UPDATE`, id).Scan(&b.ID, &b.ImportJobID, &b.ProposalID,
		&b.ActorID, &b.Status, &b.CreatedAt, &b.RolledBackAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: change_batch=%s", ErrProposalNotFound, id)
	}
	return &b, err
}

func (r *Repository) ListBatchRevisions(ctx context.Context, tx pgx.Tx, batchID uuid.UUID) ([]BatchRevision, error) {
	rows, err := r.q(tx).Query(ctx, `SELECT id,page_id,parent_revision_id,created_at FROM revision
		WHERE change_batch_id=$1 ORDER BY created_at,id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BatchRevision
	for rows.Next() {
		var rev BatchRevision
		if err := rows.Scan(
			&rev.ID, &rev.PageID, &rev.ParentRevisionID, &rev.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

func (r *Repository) HasPageRevisionAfterBatch(
	ctx context.Context,
	tx pgx.Tx,
	pageID, lastBatchRevisionID, batchID uuid.UUID,
) (bool, error) {
	var exists bool
	err := r.q(tx).QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM revision later
		JOIN revision anchor ON anchor.id=$2
		WHERE later.page_id=$1
		  AND later.change_batch_id IS DISTINCT FROM $3
		  AND (later.created_at,later.id) > (anchor.created_at,anchor.id)
	)`, pageID, lastBatchRevisionID, batchID).Scan(&exists)
	return exists, err
}

func (r *Repository) CurrentPageRevisionID(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) (*uuid.UUID, error) {
	var id *uuid.UUID
	if err := r.q(tx).QueryRow(ctx, `SELECT current_revision_id FROM page WHERE id=$1`, pageID).Scan(&id); err != nil {
		return nil, err
	}
	return id, nil
}

func (r *Repository) ListBatchClaimIDs(ctx context.Context, tx pgx.Tx, batchID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.q(tx).Query(ctx, `SELECT id FROM claim WHERE change_batch_id=$1 ORDER BY created_at,id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListBatchAuditEvents returns the immutable batch ledger in reverse execution
// order so callers can apply exact inverse operations.
func (r *Repository) ListBatchAuditEvents(
	ctx context.Context,
	tx pgx.Tx,
	batchID uuid.UUID,
) ([]BatchAuditEvent, error) {
	rows, err := r.q(tx).Query(ctx, `SELECT id,event_type,aggregate_type,
		aggregate_id,payload_json,created_at
		FROM audit_event
		WHERE change_batch_id=$1
		ORDER BY created_at DESC,id DESC`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []BatchAuditEvent{}
	for rows.Next() {
		var event BatchAuditEvent
		if err := rows.Scan(
			&event.ID, &event.EventType, &event.AggregateType,
			&event.AggregateID, &event.Payload, &event.CreatedAt,
		); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (r *Repository) HasApprovalEvidence(ctx context.Context, tx pgx.Tx, proposalID uuid.UUID) (bool, error) {
	var approved bool
	err := r.q(tx).QueryRow(ctx, `SELECT
		COALESCE((policy_decision_json->>'auto_approve')::boolean,false)
		OR EXISTS (SELECT 1 FROM review_task WHERE proposal_id=$1 AND status='approved')
		FROM proposal WHERE id=$1`, proposalID).Scan(&approved)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrProposalNotFound
	}
	return approved, err
}
