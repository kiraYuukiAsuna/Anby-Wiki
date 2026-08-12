package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	ProjectionRelatedPages               = "related_pages"
	maxCollectionCandidatesPerCollection = 24
)

// RelatedReason is intentionally small and presentation-neutral. The reader
// can explain a recommendation without exposing its internal numeric weights.
type RelatedReason struct {
	Type  string `json:"type"`
	Label string `json:"label,omitempty"`
}

// RelatedPagesBuilder materializes deterministic article recommendations from
// disposable/current projections and authoritative Collection membership.
// It never writes Page, Entity, Claim or Collection state.
type RelatedPagesBuilder struct {
	pool *pgxpool.Pool
}

func NewRelatedPagesBuilder(pool *pgxpool.Pool) *RelatedPagesBuilder {
	return &RelatedPagesBuilder{pool: pool}
}

func (b *RelatedPagesBuilder) Type() string { return ProjectionRelatedPages }

type relatedCandidate struct {
	pageID  uuid.UUID
	score   int
	reasons []RelatedReason
	seen    map[string]struct{}
}

func (c *relatedCandidate) add(score int, reason RelatedReason) {
	key := reason.Type + "\x00" + reason.Label
	if _, exists := c.seen[key]; exists {
		return
	}
	c.seen[key] = struct{}{}
	c.score += score
	c.reasons = append(c.reasons, reason)
}

func (b *RelatedPagesBuilder) Rebuild(
	ctx context.Context,
	tx pgx.Tx,
	pageID, revisionID uuid.UUID,
) error {
	if _, err := tx.Exec(
		ctx, `DELETE FROM page_related_projection WHERE source_page_id=$1`, pageID,
	); err != nil {
		return fmt.Errorf("projection: clear related pages for %s: %w", pageID, err)
	}

	candidates := make(map[uuid.UUID]*relatedCandidate)
	add := func(targetID uuid.UUID, score int, reason RelatedReason) {
		if targetID == uuid.Nil || targetID == pageID {
			return
		}
		candidate := candidates[targetID]
		if candidate == nil {
			candidate = &relatedCandidate{
				pageID: targetID,
				seen:   make(map[string]struct{}),
			}
			candidates[targetID] = candidate
		}
		candidate.add(score, reason)
	}

	if err := collectRelatedLinks(ctx, tx, pageID, revisionID, add); err != nil {
		return err
	}
	if err := collectRelatedCollections(ctx, tx, pageID, add); err != nil {
		return err
	}
	if err := collectRelatedEntities(ctx, tx, pageID, add); err != nil {
		return err
	}

	ordered := make([]*relatedCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		ordered = append(ordered, candidate)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].score != ordered[j].score {
			return ordered[i].score > ordered[j].score
		}
		return ordered[i].pageID.String() < ordered[j].pageID.String()
	})
	if len(ordered) > 12 {
		ordered = ordered[:12]
	}
	for _, candidate := range ordered {
		sort.Slice(candidate.reasons, func(i, j int) bool {
			left, right := relatedReasonRank(candidate.reasons[i].Type),
				relatedReasonRank(candidate.reasons[j].Type)
			if left != right {
				return left < right
			}
			return candidate.reasons[i].Label < candidate.reasons[j].Label
		})
		reasons, err := json.Marshal(candidate.reasons)
		if err != nil {
			return fmt.Errorf("projection: encode related-page reasons: %w", err)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO page_related_projection
			(source_page_id,source_revision_id,target_page_id,score,reasons_json)
			VALUES ($1,$2,$3,$4,$5)`,
			pageID, revisionID, candidate.pageID, candidate.score, reasons,
		); err != nil {
			return fmt.Errorf(
				"projection: write related page %s -> %s: %w",
				pageID, candidate.pageID, err,
			)
		}
	}
	return nil
}

func relatedReasonRank(reasonType string) int {
	switch reasonType {
	case "entity_relation":
		return 0
	case "same_entity":
		return 1
	case "reciprocal_link":
		return 2
	case "page_link":
		return 3
	case "collection":
		return 4
	case "backlink":
		return 5
	default:
		return 6
	}
}

func (b *RelatedPagesBuilder) HandleEvent(ctx context.Context, event Event) error {
	return HandleRebuildEvent(ctx, b.pool, b, event.AggregateID)
}

type relatedAdder func(uuid.UUID, int, RelatedReason)

func collectRelatedLinks(
	ctx context.Context,
	tx pgx.Tx,
	pageID, revisionID uuid.UUID,
	add relatedAdder,
) error {
	rows, err := tx.Query(ctx, `
		WITH linked AS (
		  SELECT link.target_page_id AS target_page_id,'page_link'::text AS reason_type
		  FROM page_link_projection link
		  WHERE link.source_page_id=$1
		    AND link.source_revision_id=$2
		    AND link.resolution_status='resolved'
		  UNION
		  SELECT link.source_page_id,'backlink'
		  FROM page_link_projection link
		  JOIN page link_source ON link_source.id=link.source_page_id
		  WHERE link.target_page_id=$1
		    AND link.source_revision_id=link_source.current_revision_id
		    AND link.resolution_status='resolved'
		)
		SELECT DISTINCT linked.target_page_id,linked.reason_type,
		       CASE linked.reason_type
		         WHEN 'page_link' THEN EXISTS (
		           SELECT 1
		           FROM page_link_projection reverse_link
		           JOIN page reverse_page ON reverse_page.id=reverse_link.source_page_id
		           WHERE reverse_link.source_page_id=linked.target_page_id
		             AND reverse_link.target_page_id=$1
		             AND reverse_link.resolution_status='resolved'
		             AND reverse_link.source_revision_id=reverse_page.current_revision_id
		         )
		         ELSE EXISTS (
		           SELECT 1
		           FROM page_link_projection forward_link
		           WHERE forward_link.source_page_id=$1
		             AND forward_link.target_page_id=linked.target_page_id
		             AND forward_link.resolution_status='resolved'
		             AND forward_link.source_revision_id=$2
		         )
		       END AS reciprocal
		FROM linked
		JOIN page target ON target.id=linked.target_page_id
		JOIN page source ON source.id=$1
		WHERE linked.target_page_id<>$1
		  AND target.deleted_at IS NULL
		  AND target.current_revision_id IS NOT NULL
		  AND target.wiki_id=source.wiki_id`, pageID, revisionID)
	if err != nil {
		return fmt.Errorf("projection: query related page links: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var targetID uuid.UUID
		var reasonType string
		var reciprocal bool
		if err := rows.Scan(&targetID, &reasonType, &reciprocal); err != nil {
			return fmt.Errorf("projection: scan related page link: %w", err)
		}
		weight := 4
		if reasonType == "backlink" {
			weight = 2
		}
		add(targetID, weight, RelatedReason{Type: reasonType})
		if reciprocal {
			add(targetID, 2, RelatedReason{Type: "reciprocal_link"})
		}
	}
	return rows.Err()
}

func collectRelatedCollections(
	ctx context.Context,
	tx pgx.Tx,
	pageID uuid.UUID,
	add relatedAdder,
) error {
	rows, err := tx.Query(ctx, `
		WITH effective_members AS (
		  SELECT membership.collection_id,membership.page_id
		  FROM collection_membership membership
		  WHERE membership.member_type='page'
		  UNION
		  SELECT membership.collection_id,page.id
		  FROM collection_membership membership
		  JOIN page ON page.primary_entity_id=membership.entity_id
		  WHERE membership.member_type='entity'
		    AND page.deleted_at IS NULL
		    AND page.current_revision_id IS NOT NULL
		), source_collections AS (
		  SELECT collection_id
		  FROM effective_members
		  WHERE page_id=$1
		), ranked_targets AS (
		  SELECT target.page_id,collection.title,
		         row_number() OVER (
		           PARTITION BY target.collection_id
		           ORDER BY target.page_id
		         ) AS collection_rank
		  FROM source_collections source
		JOIN effective_members target
		  ON target.collection_id=source.collection_id
		 AND target.page_id<>$1
		JOIN collection ON collection.id=source.collection_id
		JOIN page target_page ON target_page.id=target.page_id
		JOIN page source_page ON source_page.id=$1
		WHERE target_page.deleted_at IS NULL
		  AND target_page.current_revision_id IS NOT NULL
		  AND target_page.wiki_id=source_page.wiki_id
		)
		SELECT page_id,title
		FROM ranked_targets
		WHERE collection_rank<=$2
		ORDER BY title,page_id`, pageID, maxCollectionCandidatesPerCollection)
	if err != nil {
		return fmt.Errorf("projection: query shared collections: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var targetID uuid.UUID
		var title string
		if err := rows.Scan(&targetID, &title); err != nil {
			return fmt.Errorf("projection: scan shared collection: %w", err)
		}
		add(targetID, 3, RelatedReason{Type: "collection", Label: title})
	}
	return rows.Err()
}

func collectRelatedEntities(
	ctx context.Context,
	tx pgx.Tx,
	pageID uuid.UUID,
	add relatedAdder,
) error {
	var entityID *uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT primary_entity_id FROM page WHERE id=$1`, pageID).
		Scan(&entityID); err != nil {
		return fmt.Errorf("projection: query page primary entity: %w", err)
	}
	if entityID == nil {
		return nil
	}
	rows, err := tx.Query(ctx, `
		SELECT target.id,relation.reason_type,relation.reason_label
		FROM (
		  SELECT $1::uuid AS neighbor_id,'same_entity'::text AS reason_type,
		         ''::text AS reason_label,4 AS weight
		  UNION ALL
		  SELECT CASE
		           WHEN edge.subject_entity_id=$1 THEN edge.target_entity_id
		           ELSE edge.subject_entity_id
		         END,
		         'entity_relation',edge.property_name,5
		  FROM entity_edge_projection edge
		  WHERE edge.subject_entity_id=$1 OR edge.target_entity_id=$1
		) relation
		JOIN page target ON target.primary_entity_id=relation.neighbor_id
		JOIN page source ON source.id=$2
		WHERE target.id<>$2
		  AND target.wiki_id=source.wiki_id
		  AND target.deleted_at IS NULL
		  AND target.current_revision_id IS NOT NULL
		ORDER BY relation.weight DESC,relation.reason_label,target.id`,
		*entityID, pageID)
	if err != nil {
		return fmt.Errorf("projection: query related entities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var targetID uuid.UUID
		var reasonType, label string
		if err := rows.Scan(&targetID, &reasonType, &label); err != nil {
			return fmt.Errorf("projection: scan related entity: %w", err)
		}
		weight := 5
		if reasonType == "same_entity" {
			weight = 4
		}
		add(targetID, weight, RelatedReason{Type: reasonType, Label: label})
	}
	return rows.Err()
}
