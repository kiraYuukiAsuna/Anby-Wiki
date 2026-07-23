package governance

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type entityMergeRepairReference struct {
	Kind       string
	PageID     uuid.UUID
	RevisionID uuid.UUID
	BlockID    string
	NodeID     string
	OldID      uuid.UUID
	NewID      uuid.UUID
}

func (r *Repository) entityMergeRepairStatus(
	ctx context.Context,
	tx pgx.Tx,
	mergeID uuid.UUID,
) (string, error) {
	var status string
	err := r.q(tx).QueryRow(ctx, `
		SELECT status FROM entity_merge WHERE id=$1 FOR SHARE`, mergeID).Scan(&status)
	if err == pgx.ErrNoRows {
		return "", ErrInvalidProposal
	}
	if err != nil {
		return "", fmt.Errorf("governance: check Entity merge: %w", err)
	}
	return status, nil
}

func (r *Repository) listEntityMergeRepairReferences(
	ctx context.Context,
	tx pgx.Tx,
	mergeID uuid.UUID,
) ([]entityMergeRepairReference, error) {
	rows, err := r.q(tx).Query(ctx, `
		SELECT kind,page_id,revision_id,block_id::text,node_id,old_id,new_id
		FROM (
			SELECT 'entity'::text AS kind,u.page_id,u.revision_id,u.block_id,u.node_id,
			       m.source_entity_id AS old_id,m.target_entity_id AS new_id
			FROM entity_merge m
			JOIN entity_mention_projection u ON u.entity_id=m.source_entity_id
			JOIN page p ON p.id=u.page_id AND p.current_revision_id=u.revision_id
			WHERE m.id=$1 AND m.status='applied'
			UNION ALL
			SELECT 'claim'::text,u.page_id,u.revision_id,u.block_id,u.node_id,
			       m.old_claim_id,m.new_claim_id
			FROM entity_merge_claim_map m
			JOIN entity_merge em ON em.id=m.merge_id AND em.status='applied'
			JOIN claim_usage u ON u.claim_id=m.old_claim_id
			JOIN page p ON p.id=u.page_id AND p.current_revision_id=u.revision_id
			WHERE m.merge_id=$1 AND u.node_id NOT LIKE 'component:%'
		) refs
		ORDER BY page_id,block_id,node_id,kind`, mergeID)
	if err != nil {
		return nil, fmt.Errorf("governance: query Entity merge repair references: %w", err)
	}
	defer rows.Close()
	var result []entityMergeRepairReference
	for rows.Next() {
		var ref entityMergeRepairReference
		if err := rows.Scan(&ref.Kind, &ref.PageID, &ref.RevisionID, &ref.BlockID,
			&ref.NodeID, &ref.OldID, &ref.NewID); err != nil {
			return nil, err
		}
		result = append(result, ref)
	}
	return result, rows.Err()
}

func (r *Repository) currentRepairAST(
	ctx context.Context,
	tx pgx.Tx,
	pageID, revisionID uuid.UUID,
) (json.RawMessage, error) {
	var raw json.RawMessage
	err := tx.QueryRow(ctx, `
		SELECT s.ast_json
		FROM page p
		JOIN revision rev ON rev.id=p.current_revision_id
		JOIN content_snapshot s ON s.id=rev.content_snapshot_id
		WHERE p.id=$1 AND p.current_revision_id=$2
		FOR SHARE OF p`, pageID, revisionID).Scan(&raw)
	if err == pgx.ErrNoRows {
		return nil, ErrRepairProjectionStale
	}
	if err != nil {
		return nil, fmt.Errorf("governance: read current repair AST: %w", err)
	}
	return raw, nil
}
