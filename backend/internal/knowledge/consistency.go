package knowledge

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var (
	ErrInvalidConsistencyFilter = errors.New("knowledge: 事实一致性筛选非法")
	ErrInvalidConsistencyCursor = errors.New("knowledge: 事实一致性 cursor 非法")
)

const (
	ConsistencySingleValueConflict = "single_value_conflict"
	ConsistencyMultiplePreferred   = "multiple_preferred_values"
	ConsistencyVerifiedNoSupport   = "verified_without_support"
	ConsistencyMergedTarget        = "merged_entity_target"
	ConsistencyEvidenceDisagree    = "evidence_disagreement"

	ConsistencyWarning = "warning"
	ConsistencyError   = "error"

	ConsistencyOpen     = "open"
	ConsistencyResolved = "resolved"
)

var validConsistencyTypes = map[string]bool{
	ConsistencySingleValueConflict: true,
	ConsistencyMultiplePreferred:   true,
	ConsistencyVerifiedNoSupport:   true,
	ConsistencyMergedTarget:        true,
	ConsistencyEvidenceDisagree:    true,
}

type FactConsistencyIssue struct {
	ID              uuid.UUID       `json:"id"`
	WikiID          uuid.UUID       `json:"wiki_id"`
	SubjectEntityID uuid.UUID       `json:"subject_entity_id"`
	SubjectLabel    string          `json:"subject_label"`
	PropertyID      *uuid.UUID      `json:"property_id"`
	PropertyKey     *string         `json:"property_key"`
	PropertyName    *string         `json:"property_name"`
	IssueKey        string          `json:"issue_key"`
	IssueType       string          `json:"issue_type"`
	Severity        string          `json:"severity"`
	ClaimIDs        []uuid.UUID     `json:"claim_ids"`
	Details         json.RawMessage `json:"details"`
	Status          string          `json:"status"`
	DetectedAt      time.Time       `json:"detected_at"`
	LastCheckedAt   time.Time       `json:"last_checked_at"`
	ResolvedAt      *time.Time      `json:"resolved_at"`
}

type FactConsistencyPage struct {
	Items      []FactConsistencyIssue `json:"items"`
	NextCursor *string                `json:"next_cursor"`
}

type FactConsistencyListParams struct {
	WikiID    uuid.UUID
	Status    string
	IssueType string
	Cursor    string
	Limit     int
}

type FactConsistencyScanResult struct {
	ScannedSubjects int `json:"scanned_subjects"`
	OpenIssues      int `json:"open_issues"`
	ResolvedIssues  int `json:"resolved_issues"`
}

type consistencyCandidate struct {
	PropertyID *uuid.UUID
	IssueKey   string
	IssueType  string
	Severity   string
	ClaimIDs   []uuid.UUID
	Details    json.RawMessage
}

type FactConsistencyService struct {
	repo *Repository
	txm  *db.TxManager
	ids  *id.Generator
}

func NewFactConsistencyService(
	repo *Repository,
	txm *db.TxManager,
	ids *id.Generator,
) *FactConsistencyService {
	return &FactConsistencyService{repo: repo, txm: txm, ids: ids}
}

// RebuildSubject derives the complete issue set for one Entity from current
// authoritative Claim/ClaimSource rows. Missing candidates resolve old issues;
// no Claim is changed by this projection.
func (s *FactConsistencyService) RebuildSubject(
	ctx context.Context,
	subjectID uuid.UUID,
) (open, resolved int, err error) {
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1,0))`,
			"anby:fact-consistency:"+subjectID.String(),
		); err != nil {
			return err
		}
		var wikiID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT wiki_id FROM entity
			WHERE id=$1 AND status IN ('active','merged')`,
			subjectID,
		).Scan(&wikiID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrEntityNotFound
			}
			return err
		}
		candidates, err := s.detect(ctx, tx, subjectID)
		if err != nil {
			return err
		}
		keys := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			keys = append(keys, candidate.IssueKey)
			issueID, err := s.ids.New()
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO fact_consistency_issue
				(id,wiki_id,subject_entity_id,property_id,issue_key,
				 issue_type,severity,claim_ids,details_json,status)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'open')
				ON CONFLICT (wiki_id,issue_key) DO UPDATE SET
					property_id=EXCLUDED.property_id,
					issue_type=EXCLUDED.issue_type,
					severity=EXCLUDED.severity,
					claim_ids=EXCLUDED.claim_ids,
					details_json=EXCLUDED.details_json,
					status='open',
					last_checked_at=now(),
					resolved_at=NULL`,
				issueID, wikiID, subjectID, candidate.PropertyID,
				candidate.IssueKey, candidate.IssueType,
				candidate.Severity, candidate.ClaimIDs,
				candidate.Details,
			); err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE fact_consistency_issue
			SET status='resolved',last_checked_at=now(),resolved_at=now()
			WHERE wiki_id=$1 AND subject_entity_id=$2 AND status='open'
			  AND NOT (issue_key=ANY($3::text[]))`,
			wikiID, subjectID, keys,
		)
		if err != nil {
			return err
		}
		open = len(candidates)
		resolved = int(tag.RowsAffected())
		return nil
	})
	return open, resolved, err
}

func (s *FactConsistencyService) detect(
	ctx context.Context,
	tx pgx.Tx,
	subjectID uuid.UUID,
) ([]consistencyCandidate, error) {
	candidates := []consistencyCandidate{}
	rows, err := tx.Query(ctx, `SELECT c.property_id,
		array_agg(c.id ORDER BY c.id)
		FROM claim c JOIN property p ON p.id=c.property_id
		WHERE c.subject_entity_id=$1 AND c.status='published'
		  AND p.is_multivalued=false
		GROUP BY c.property_id HAVING count(*)>1`, subjectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var propertyID uuid.UUID
		var claimIDs []uuid.UUID
		if err := rows.Scan(&propertyID, &claimIDs); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, consistencyCandidate{
			PropertyID: &propertyID,
			IssueKey: fmt.Sprintf("%s:%s:%s",
				ConsistencySingleValueConflict, subjectID, propertyID),
			IssueType: ConsistencySingleValueConflict,
			Severity:  ConsistencyError, ClaimIDs: claimIDs,
			Details: mustConsistencyDetails(map[string]any{
				"published_claim_count": len(claimIDs),
			}),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT c.property_id,
		array_agg(c.id ORDER BY c.id)
		FROM claim c JOIN property p ON p.id=c.property_id
		WHERE c.subject_entity_id=$1 AND c.status='published'
		  AND c.rank='preferred' AND p.is_multivalued=true
		GROUP BY c.property_id HAVING count(*)>1`, subjectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var propertyID uuid.UUID
		var claimIDs []uuid.UUID
		if err := rows.Scan(&propertyID, &claimIDs); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, consistencyCandidate{
			PropertyID: &propertyID,
			IssueKey: fmt.Sprintf("%s:%s:%s",
				ConsistencyMultiplePreferred, subjectID, propertyID),
			IssueType: ConsistencyMultiplePreferred,
			Severity:  ConsistencyWarning, ClaimIDs: claimIDs,
			Details: mustConsistencyDetails(map[string]any{
				"preferred_claim_count": len(claimIDs),
			}),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	rows, err = tx.Query(ctx, `SELECT c.id,c.property_id,
		CASE
		  WHEN c.verification_status='human_verified'
		       AND NOT EXISTS (
		         SELECT 1 FROM claim_source cs
		         WHERE cs.claim_id=c.id AND cs.support_type='supports'
		       ) THEN $2
		  WHEN c.target_entity_id IS NOT NULL
		       AND EXISTS (
		         SELECT 1 FROM entity target
		         WHERE target.id=c.target_entity_id AND target.status='merged'
		       ) THEN $3
		  WHEN c.verification_status<>'disputed'
		       AND EXISTS (
		         SELECT 1 FROM claim_source cs
		         WHERE cs.claim_id=c.id AND cs.support_type='supports'
		       )
		       AND EXISTS (
		         SELECT 1 FROM claim_source cs
		         WHERE cs.claim_id=c.id AND cs.support_type='contradicts'
		       ) THEN $4
		END AS issue_type
		FROM claim c
		WHERE c.subject_entity_id=$1 AND c.status='published'
		  AND (
		    (c.verification_status='human_verified' AND NOT EXISTS (
		      SELECT 1 FROM claim_source cs
		      WHERE cs.claim_id=c.id AND cs.support_type='supports'
		    ))
		    OR (c.target_entity_id IS NOT NULL AND EXISTS (
		      SELECT 1 FROM entity target
		      WHERE target.id=c.target_entity_id AND target.status='merged'
		    ))
		    OR (c.verification_status<>'disputed'
		        AND EXISTS (
		          SELECT 1 FROM claim_source cs
		          WHERE cs.claim_id=c.id AND cs.support_type='supports'
		        )
		        AND EXISTS (
		          SELECT 1 FROM claim_source cs
		          WHERE cs.claim_id=c.id AND cs.support_type='contradicts'
		        ))
		  )
		ORDER BY c.id`,
		subjectID, ConsistencyVerifiedNoSupport,
		ConsistencyMergedTarget, ConsistencyEvidenceDisagree,
	)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var claimID, propertyID uuid.UUID
		var issueType string
		if err := rows.Scan(&claimID, &propertyID, &issueType); err != nil {
			rows.Close()
			return nil, err
		}
		severity := ConsistencyWarning
		if issueType == ConsistencyMergedTarget {
			severity = ConsistencyError
		}
		candidates = append(candidates, consistencyCandidate{
			PropertyID: &propertyID,
			IssueKey:   fmt.Sprintf("%s:%s", issueType, claimID),
			IssueType:  issueType, Severity: severity,
			ClaimIDs: []uuid.UUID{claimID},
			Details: mustConsistencyDetails(map[string]any{
				"claim_id": claimID,
			}),
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	return candidates, nil
}

func mustConsistencyDetails(value map[string]any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

func (s *FactConsistencyService) ScanWiki(
	ctx context.Context,
	wikiID uuid.UUID,
) (*FactConsistencyScanResult, error) {
	rows, err := s.repo.pool.Query(ctx, `SELECT id FROM entity
		WHERE wiki_id=$1 AND status IN ('active','merged')
		ORDER BY id`, wikiID)
	if err != nil {
		return nil, err
	}
	var subjectIDs []uuid.UUID
	for rows.Next() {
		var subjectID uuid.UUID
		if err := rows.Scan(&subjectID); err != nil {
			rows.Close()
			return nil, err
		}
		subjectIDs = append(subjectIDs, subjectID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	result := &FactConsistencyScanResult{}
	for _, subjectID := range subjectIDs {
		open, resolved, err := s.RebuildSubject(ctx, subjectID)
		if err != nil {
			return result, err
		}
		result.ScannedSubjects++
		result.OpenIssues += open
		result.ResolvedIssues += resolved
	}
	return result, nil
}

type consistencyCursor struct {
	CheckedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

func (s *FactConsistencyService) List(
	ctx context.Context,
	params FactConsistencyListParams,
) (*FactConsistencyPage, error) {
	status := strings.TrimSpace(params.Status)
	if status == "" {
		status = ConsistencyOpen
	}
	if status != ConsistencyOpen &&
		status != ConsistencyResolved && status != "all" {
		return nil, ErrInvalidConsistencyFilter
	}
	issueType := strings.TrimSpace(params.IssueType)
	if issueType != "" && !validConsistencyTypes[issueType] {
		return nil, ErrInvalidConsistencyFilter
	}
	if params.Limit <= 0 {
		params.Limit = 30
	}
	if params.Limit > 100 {
		params.Limit = 100
	}
	var afterTime *time.Time
	var afterID uuid.UUID
	if params.Cursor != "" {
		raw, err := base64.RawURLEncoding.DecodeString(params.Cursor)
		if err != nil {
			return nil, ErrInvalidConsistencyCursor
		}
		var cursor consistencyCursor
		if err := json.Unmarshal(raw, &cursor); err != nil ||
			cursor.CheckedAt.IsZero() || cursor.ID == uuid.Nil {
			return nil, ErrInvalidConsistencyCursor
		}
		afterTime, afterID = &cursor.CheckedAt, cursor.ID
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT
		i.id,i.wiki_id,i.subject_entity_id,
		COALESCE(label.label,entity.canonical_key),
		i.property_id,p.property_key,p.name,i.issue_key,i.issue_type,
		i.severity,i.claim_ids,i.details_json,i.status,
		i.detected_at,i.last_checked_at,i.resolved_at
		FROM fact_consistency_issue i
		JOIN entity ON entity.id=i.subject_entity_id
		LEFT JOIN property p ON p.id=i.property_id
		LEFT JOIN LATERAL (
		  SELECT el.label FROM entity_label el
		  WHERE el.entity_id=entity.id
		  ORDER BY el.is_primary DESC,el.language,el.label LIMIT 1
		) label ON true
		WHERE i.wiki_id=$1
		  AND ($2='all' OR i.status=$2)
		  AND ($3='' OR i.issue_type=$3)
		  AND ($4::timestamptz IS NULL
		       OR (i.last_checked_at,i.id)<($4,$5))
		ORDER BY i.last_checked_at DESC,i.id DESC
		LIMIT $6`,
		params.WikiID, status, issueType, afterTime, afterID,
		params.Limit+1,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &FactConsistencyPage{
		Items: make([]FactConsistencyIssue, 0, params.Limit),
	}
	for rows.Next() {
		var item FactConsistencyIssue
		if err := rows.Scan(
			&item.ID, &item.WikiID, &item.SubjectEntityID,
			&item.SubjectLabel, &item.PropertyID, &item.PropertyKey,
			&item.PropertyName, &item.IssueKey, &item.IssueType,
			&item.Severity, &item.ClaimIDs, &item.Details,
			&item.Status, &item.DetectedAt, &item.LastCheckedAt,
			&item.ResolvedAt,
		); err != nil {
			return nil, err
		}
		result.Items = append(result.Items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > params.Limit {
		last := result.Items[params.Limit-1]
		raw, _ := json.Marshal(consistencyCursor{
			CheckedAt: last.LastCheckedAt, ID: last.ID,
		})
		cursor := base64.RawURLEncoding.EncodeToString(raw)
		result.NextCursor = &cursor
		result.Items = result.Items[:params.Limit]
	}
	return result, nil
}
