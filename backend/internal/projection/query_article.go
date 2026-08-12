package projection

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PageReference is one unique Citation used by the current Revision. Number is
// stable across full and sectioned delivery because it follows document_order.
type PageReference struct {
	Number          int
	CitationID      uuid.UUID
	OccurrenceCount int
	FirstBlockID    uuid.UUID
	FirstNodeID     string
	Occurrences     []PageReferenceOccurrence
	SourceVersionID uuid.UUID
	SourceID        uuid.UUID
	SourceType      string
	SourceTitle     string
	Author          *string
	Publisher       *string
	PublishedAt     *time.Time
	SourceMetadata  json.RawMessage
	VersionHash     string
	FetchedAt       time.Time
	Locator         json.RawMessage
	Quotation       *string
	ExternalURL     *string
}

// PageReferenceOccurrence identifies one inline use of a Citation. Keeping
// every occurrence lets the bibliography provide Wikipedia-style a/b/c
// backlinks instead of only returning to the first marker.
type PageReferenceOccurrence struct {
	BlockID uuid.UUID `json:"block_id"`
	NodeID  string    `json:"node_id"`
}

type PageReferences struct {
	Ready      bool
	RevisionID *uuid.UUID
	Items      []PageReference
}

// References returns a bibliography projection for the current Revision. A
// not-yet-built projection is represented as ready=false instead of silently
// returning a misleading empty bibliography.
func (q *Queries) References(
	ctx context.Context,
	pageID uuid.UUID,
) (*PageReferences, error) {
	if err := q.ensurePageExists(ctx, pageID); err != nil {
		return nil, err
	}
	result := &PageReferences{Items: []PageReference{}}
	if err := q.pool.QueryRow(ctx, `
		SELECT current_revision_id
		FROM page
		WHERE id=$1 AND deleted_at IS NULL`, pageID).Scan(&result.RevisionID); err != nil {
		return nil, fmt.Errorf("projection: query reference revision: %w", err)
	}
	if result.RevisionID == nil {
		return result, nil
	}
	if err := q.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1
		FROM projection_state
		WHERE aggregate_type='page'
		  AND aggregate_id=$1
		  AND projection_type=$2
		  AND source_revision_id=$3
		  AND status='ok'
	)`, pageID, ProjectionCitationUsage, *result.RevisionID).
		Scan(&result.Ready); err != nil {
		return nil, fmt.Errorf("projection: query reference readiness: %w", err)
	}
	if !result.Ready {
		return result, nil
	}
	rows, err := q.pool.Query(ctx, `
		WITH citation_rank AS (
		  SELECT citation_id,min(document_order) AS first_order,
		         count(*)::int AS occurrence_count,
		         (array_agg(block_id ORDER BY document_order))[1] AS first_block_id,
		         (array_agg(node_id ORDER BY document_order))[1] AS first_node_id,
		         jsonb_agg(jsonb_build_object(
		           'block_id',block_id,'node_id',node_id
		         ) ORDER BY document_order) AS occurrences
		  FROM citation_usage
		  WHERE page_id=$1 AND revision_id=$2
		  GROUP BY citation_id
		)
		SELECT row_number() OVER (ORDER BY rank.first_order)::int,
		       citation.id,rank.occurrence_count,
		       rank.first_block_id,rank.first_node_id,rank.occurrences,
		       version.id,source.id,source.source_type,source.title,
		       source.author,source.publisher,source.published_at,
		       source.metadata_json,version.version_hash,version.fetched_at,
		       COALESCE(citation.locator_json,'{}'::jsonb),citation.quotation,
		       COALESCE(resource.canonical_url,resource.original_url)
		FROM citation_rank rank
		JOIN citation ON citation.id=rank.citation_id
		JOIN source_version version ON version.id=citation.source_version_id
		JOIN source ON source.id=version.source_id
		LEFT JOIN external_resource resource ON resource.id=source.external_resource_id
		ORDER BY rank.first_order`, pageID, *result.RevisionID)
	if err != nil {
		return nil, fmt.Errorf("projection: query page references: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item PageReference
		var occurrences json.RawMessage
		if err := rows.Scan(
			&item.Number, &item.CitationID, &item.OccurrenceCount,
			&item.FirstBlockID, &item.FirstNodeID,
			&occurrences,
			&item.SourceVersionID, &item.SourceID, &item.SourceType,
			&item.SourceTitle, &item.Author, &item.Publisher,
			&item.PublishedAt, &item.SourceMetadata, &item.VersionHash,
			&item.FetchedAt, &item.Locator, &item.Quotation,
			&item.ExternalURL,
		); err != nil {
			return nil, fmt.Errorf("projection: scan page reference: %w", err)
		}
		if err := json.Unmarshal(occurrences, &item.Occurrences); err != nil {
			return nil, fmt.Errorf("projection: decode reference occurrences: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: iterate page references: %w", err)
	}
	return result, nil
}

type RelatedPage struct {
	PageID       uuid.UUID
	DisplayTitle string
	Score        int
	Reasons      []RelatedReason
}

type RelatedPageList struct {
	Ready      bool
	RevisionID *uuid.UUID
	Items      []RelatedPage
}

func (q *Queries) RelatedPages(
	ctx context.Context,
	pageID uuid.UUID,
) (*RelatedPageList, error) {
	if err := q.ensurePageExists(ctx, pageID); err != nil {
		return nil, err
	}
	result := &RelatedPageList{Items: []RelatedPage{}}
	if err := q.pool.QueryRow(ctx, `
		SELECT current_revision_id
		FROM page
		WHERE id=$1 AND deleted_at IS NULL`, pageID).Scan(&result.RevisionID); err != nil {
		return nil, fmt.Errorf("projection: query related-page revision: %w", err)
	}
	if result.RevisionID == nil {
		return result, nil
	}
	if err := q.pool.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM projection_state
		WHERE aggregate_type='page' AND aggregate_id=$1
		  AND projection_type=$2 AND source_revision_id=$3 AND status='ok'
	)`, pageID, ProjectionRelatedPages, *result.RevisionID).
		Scan(&result.Ready); err != nil {
		return nil, fmt.Errorf("projection: query related-page readiness: %w", err)
	}
	if !result.Ready {
		return result, nil
	}
	rows, err := q.pool.Query(ctx, `
		SELECT related.target_page_id,target.display_title,
		       related.score,related.reasons_json
		FROM page_related_projection related
		JOIN page target ON target.id=related.target_page_id
		WHERE related.source_page_id=$1
		  AND related.source_revision_id=$2
		  AND target.deleted_at IS NULL
		  AND target.current_revision_id IS NOT NULL
		ORDER BY related.score DESC,lower(target.display_title),target.id
		LIMIT 12`, pageID, *result.RevisionID)
	if err != nil {
		return nil, fmt.Errorf("projection: query related pages: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var item RelatedPage
		var reasons json.RawMessage
		if err := rows.Scan(
			&item.PageID, &item.DisplayTitle, &item.Score, &reasons,
		); err != nil {
			return nil, fmt.Errorf("projection: scan related page: %w", err)
		}
		if err := json.Unmarshal(reasons, &item.Reasons); err != nil {
			return nil, fmt.Errorf("projection: decode related-page reasons: %w", err)
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("projection: iterate related pages: %w", err)
	}
	return result, nil
}
