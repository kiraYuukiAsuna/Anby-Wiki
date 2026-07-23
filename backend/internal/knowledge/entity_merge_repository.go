package knowledge

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) getActorType(ctx context.Context, tx pgx.Tx, actorID uuid.UUID) (string, error) {
	var actorType, status string
	err := r.q(tx).QueryRow(ctx, `SELECT actor_type,status FROM actor WHERE id=$1`, actorID).
		Scan(&actorType, &status)
	if errors.Is(err, pgx.ErrNoRows) || status != "active" {
		return "", ErrEntityMergeActorOnly
	}
	if err != nil {
		return "", fmt.Errorf("knowledge: query merge actor: %w", err)
	}
	return actorType, nil
}

func (r *Repository) insertEntityMerge(ctx context.Context, tx pgx.Tx, merge *EntityMerge) error {
	return tx.QueryRow(ctx, `INSERT INTO entity_merge
		(id,source_entity_id,target_entity_id,actor_id,status,reason)
		VALUES ($1,$2,$3,$4,'applied',$5) RETURNING created_at`,
		merge.ID, merge.SourceEntityID, merge.TargetEntityID, merge.ActorID, merge.Reason).
		Scan(&merge.CreatedAt)
}

func (r *Repository) getAppliedMergeBySource(
	ctx context.Context,
	tx pgx.Tx,
	sourceID uuid.UUID,
) (*EntityMerge, error) {
	var merge EntityMerge
	err := r.q(tx).QueryRow(ctx, `SELECT id,source_entity_id,target_entity_id,actor_id,status,
		reason,created_at,rolled_back_at,rolled_back_by
		FROM entity_merge WHERE source_entity_id=$1 AND status='applied'`, sourceID).
		Scan(&merge.ID, &merge.SourceEntityID, &merge.TargetEntityID, &merge.ActorID,
			&merge.Status, &merge.Reason, &merge.CreatedAt, &merge.RolledBackAt, &merge.RolledBackBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: query applied Entity merge: %w", err)
	}
	return &merge, nil
}

func (r *Repository) getEntityMergeForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	mergeID uuid.UUID,
) (*EntityMerge, error) {
	var merge EntityMerge
	err := tx.QueryRow(ctx, `SELECT id,source_entity_id,target_entity_id,actor_id,status,
		reason,created_at,rolled_back_at,rolled_back_by
		FROM entity_merge WHERE id=$1 FOR UPDATE`, mergeID).
		Scan(&merge.ID, &merge.SourceEntityID, &merge.TargetEntityID, &merge.ActorID,
			&merge.Status, &merge.Reason, &merge.CreatedAt, &merge.RolledBackAt, &merge.RolledBackBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", ErrEntityMergeNotFound, mergeID)
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: lock Entity merge: %w", err)
	}
	return &merge, nil
}

func (r *Repository) markEntityMerged(
	ctx context.Context,
	tx pgx.Tx,
	sourceID, targetID uuid.UUID,
) error {
	tag, err := tx.Exec(ctx, `UPDATE entity SET status='merged',merged_into_entity_id=$2,
		updated_at=now() WHERE id=$1 AND status='active'`, sourceID, targetID)
	if err != nil {
		return fmt.Errorf("knowledge: mark Entity merged: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrInvalidEntityMerge
	}
	return nil
}

func (r *Repository) restoreMergedEntity(
	ctx context.Context,
	tx pgx.Tx,
	sourceID, targetID uuid.UUID,
) error {
	tag, err := tx.Exec(ctx, `UPDATE entity SET status='active',merged_into_entity_id=NULL,
		updated_at=now() WHERE id=$1 AND status='merged' AND merged_into_entity_id=$2`,
		sourceID, targetID)
	if err != nil {
		return fmt.Errorf("knowledge: restore merged Entity: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrEntityMergeStale
	}
	return nil
}

func (r *Repository) insertMergeLabelMap(
	ctx context.Context,
	tx pgx.Tx,
	m EntityMergeLabelMap,
) error {
	_, err := tx.Exec(ctx, `INSERT INTO entity_merge_label_map
		(merge_id,language,source_label,target_label,action,target_is_primary)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		m.MergeID, m.Language, m.SourceLabel, m.TargetLabel, m.Action, m.TargetIsPrimary)
	return err
}

func (r *Repository) listMergeLabels(
	ctx context.Context,
	tx pgx.Tx,
	mergeID uuid.UUID,
) ([]EntityMergeLabelMap, error) {
	rows, err := r.q(tx).Query(ctx, `SELECT merge_id,language,source_label,target_label,
		action,target_is_primary FROM entity_merge_label_map
		WHERE merge_id=$1 ORDER BY language,source_label`, mergeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []EntityMergeLabelMap
	for rows.Next() {
		var item EntityMergeLabelMap
		if err := rows.Scan(&item.MergeID, &item.Language, &item.SourceLabel,
			&item.TargetLabel, &item.Action, &item.TargetIsPrimary); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) listCurrentClaimsForMerge(
	ctx context.Context,
	tx pgx.Tx,
	sourceID uuid.UUID,
) ([]Claim, error) {
	rows, err := tx.Query(ctx, `SELECT `+claimColumns+` FROM claim
		WHERE (subject_entity_id=$1 OR target_entity_id=$1)
		  AND status IN ('proposed','published')
		ORDER BY id FOR UPDATE`, sourceID)
	if err != nil {
		return nil, fmt.Errorf("knowledge: lock merge Claims: %w", err)
	}
	defer rows.Close()
	var result []Claim
	for rows.Next() {
		claim, err := scanClaim(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *claim)
	}
	return result, rows.Err()
}

func (r *Repository) listClaimSources(
	ctx context.Context,
	tx pgx.Tx,
	claimID uuid.UUID,
) ([]ClaimSource, error) {
	rows, err := r.q(tx).Query(ctx, `SELECT claim_id,citation_id,support_type,created_at
		FROM claim_source WHERE claim_id=$1 ORDER BY citation_id`, claimID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ClaimSource
	for rows.Next() {
		var source ClaimSource
		if err := rows.Scan(&source.ClaimID, &source.CitationID, &source.SupportType,
			&source.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, source)
	}
	return result, rows.Err()
}

func (r *Repository) copyClaimSource(
	ctx context.Context,
	tx pgx.Tx,
	newClaimID uuid.UUID,
	source ClaimSource,
) error {
	_, err := tx.Exec(ctx, `INSERT INTO claim_source (claim_id,citation_id,support_type)
		VALUES ($1,$2,$3)`, newClaimID, source.CitationID, source.SupportType)
	return err
}

func (r *Repository) updateClaimMergeStatus(
	ctx context.Context,
	tx pgx.Tx,
	claimID uuid.UUID,
	from, to string,
) error {
	tag, err := tx.Exec(ctx, `UPDATE claim SET status=$3 WHERE id=$1 AND status=$2`,
		claimID, from, to)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrEntityMergeStale
	}
	return nil
}

func (r *Repository) insertMergeClaimMap(
	ctx context.Context,
	tx pgx.Tx,
	m EntityMergeClaimMap,
) error {
	_, err := tx.Exec(ctx, `INSERT INTO entity_merge_claim_map
		(merge_id,old_claim_id,new_claim_id,old_status,new_status)
		VALUES ($1,$2,$3,$4,$5)`,
		m.MergeID, m.OldClaimID, m.NewClaimID, m.OldStatus, m.NewStatus)
	return err
}

func (r *Repository) listMergeClaimsForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	mergeID uuid.UUID,
) ([]EntityMergeClaimMap, error) {
	rows, err := tx.Query(ctx, `SELECT m.merge_id,m.old_claim_id,m.new_claim_id,
		m.old_status,m.new_status
		FROM entity_merge_claim_map m
		JOIN claim old_claim ON old_claim.id=m.old_claim_id
		JOIN claim new_claim ON new_claim.id=m.new_claim_id
		WHERE m.merge_id=$1 ORDER BY m.old_claim_id
		FOR UPDATE OF old_claim,new_claim`, mergeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []EntityMergeClaimMap
	for rows.Next() {
		var item EntityMergeClaimMap
		if err := rows.Scan(&item.MergeID, &item.OldClaimID, &item.NewClaimID,
			&item.OldStatus, &item.NewStatus); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func (r *Repository) getAppliedClaimMapping(
	ctx context.Context,
	tx pgx.Tx,
	oldClaimID uuid.UUID,
) (*uuid.UUID, error) {
	var newClaimID uuid.UUID
	err := r.q(tx).QueryRow(ctx, `SELECT m.new_claim_id
		FROM entity_merge_claim_map m
		JOIN entity_merge em ON em.id=m.merge_id
		WHERE m.old_claim_id=$1 AND em.status='applied'
		ORDER BY em.created_at DESC LIMIT 1`, oldClaimID).Scan(&newClaimID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("knowledge: query applied Claim merge mapping: %w", err)
	}
	return &newClaimID, nil
}

func (r *Repository) insertMergeAudit(
	ctx context.Context,
	tx pgx.Tx,
	id, actorID uuid.UUID,
	eventType string,
	mergeID uuid.UUID,
	payload []byte,
) error {
	_, err := tx.Exec(ctx, `INSERT INTO audit_event
		(id,actor_id,event_type,aggregate_type,aggregate_id,payload_json)
		VALUES ($1,$2,$3,'entity_merge',$4,$5)`,
		id, actorID, eventType, mergeID, payload)
	return err
}

func (r *Repository) insertMergeEvent(
	ctx context.Context,
	tx pgx.Tx,
	auditID, outboxID, actorID uuid.UUID,
	eventType string,
	mergeID uuid.UUID,
	payload []byte,
) error {
	if err := r.insertMergeAudit(ctx, tx, auditID, actorID, eventType, mergeID, payload); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO outbox_event
		(id,aggregate_type,aggregate_id,event_type,payload_json)
		VALUES ($1,'entity_merge',$2,$3,$4)`,
		outboxID, mergeID, eventType, payload)
	return err
}

func (r *Repository) markMergeRolledBack(
	ctx context.Context,
	tx pgx.Tx,
	mergeID, actorID uuid.UUID,
) error {
	tag, err := tx.Exec(ctx, `UPDATE entity_merge SET status='rolled_back',
		rolled_back_at=now(),rolled_back_by=$2 WHERE id=$1 AND status='applied'`,
		mergeID, actorID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrEntityMergeStale
	}
	return nil
}
