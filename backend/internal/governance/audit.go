package governance

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var (
	ErrChangeTagNotFound   = errors.New("governance: change tag 不存在")
	ErrInvalidChangeTag    = errors.New("governance: change tag 非法")
	ErrAuditEventNotFound  = errors.New("governance: audit event 不存在")
	ErrInvalidAuditCursor  = errors.New("governance: audit cursor 非法")
	ErrAuditTargetNotFound = errors.New("governance: change tag 目标不存在")
)

const (
	TagTargetRevision = "revision"
	TagTargetProposal = "proposal"
	TagTargetAudit    = "audit_event"

	DefaultAuditPageSize = 30
	MaxAuditPageSize     = 100
)

var changeTagKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

type ChangeTag struct {
	ID          uuid.UUID `json:"id"`
	TagKey      string    `json:"tag_key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
}

type AuditEventView struct {
	ID               uuid.UUID       `json:"id"`
	ActorID          uuid.UUID       `json:"actor_id"`
	ActorDisplayName string          `json:"actor_display_name"`
	EventType        string          `json:"event_type"`
	AggregateType    string          `json:"aggregate_type"`
	AggregateID      uuid.UUID       `json:"aggregate_id"`
	ChangeBatchID    *uuid.UUID      `json:"change_batch_id"`
	Payload          json.RawMessage `json:"payload"`
	Tags             []ChangeTag     `json:"tags"`
	CreatedAt        time.Time       `json:"created_at"`
}

type AuditEventPage struct {
	Items      []AuditEventView `json:"items"`
	NextCursor *string          `json:"next_cursor"`
}

type AuditListFilter struct {
	EventType     string
	AggregateType string
	ChangeBatchID *uuid.UUID
	TagKey        string
	Cursor        string
	Limit         int
}

type auditCursor struct {
	CreatedAt time.Time `json:"t"`
	ID        uuid.UUID `json:"i"`
}

type AuditService struct {
	repo   *Repository
	auth   *AuthorizationService
	txm    *db.TxManager
	ids    *id.Generator
	wikiID uuid.UUID
}

func NewAuditService(
	repo *Repository, auth *AuthorizationService, txm *db.TxManager,
	ids *id.Generator, wikiID uuid.UUID,
) *AuditService {
	return &AuditService{repo: repo, auth: auth, txm: txm, ids: ids, wikiID: wikiID}
}

func (s *AuditService) ListTags(ctx context.Context) ([]ChangeTag, error) {
	rows, err := s.repo.q(nil).Query(ctx, `SELECT id,tag_key,name,description,created_at
		FROM change_tag ORDER BY tag_key,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	tags := []ChangeTag{}
	for rows.Next() {
		var tag ChangeTag
		if err := rows.Scan(&tag.ID, &tag.TagKey, &tag.Name, &tag.Description, &tag.CreatedAt); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (s *AuditService) CreateTag(
	ctx context.Context, actorID uuid.UUID, key, name, description string,
) (*ChangeTag, error) {
	key = strings.TrimSpace(key)
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if !changeTagKeyPattern.MatchString(key) || name == "" ||
		len([]rune(name)) > 120 || len([]rune(description)) > 1000 {
		return nil, ErrInvalidChangeTag
	}
	var result *ChangeTag
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, s.wikiID, ActionManage, nil); err != nil {
			return err
		}
		tagID, err := s.ids.New()
		if err != nil {
			return err
		}
		tag := &ChangeTag{ID: tagID, TagKey: key, Name: name, Description: description}
		if err := tx.QueryRow(ctx, `INSERT INTO change_tag (id,tag_key,name,description)
			VALUES ($1,$2,$3,$4) RETURNING created_at`,
			tag.ID, tag.TagKey, tag.Name, tag.Description,
		).Scan(&tag.CreatedAt); err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23505" {
				return fmt.Errorf("%w: tag_key=%s", ErrInvalidChangeTag, key)
			}
			return err
		}
		if err := s.insertAudit(ctx, tx, actorID, "change_tag.created", "change_tag", tag.ID,
			map[string]any{"tag_key": tag.TagKey}); err != nil {
			return err
		}
		result = tag
		return nil
	})
	return result, err
}

func (s *AuditService) AssignTag(
	ctx context.Context, actorID, tagID uuid.UUID, targetType string, targetID uuid.UUID,
) (bool, error) {
	if tagID == uuid.Nil || targetID == uuid.Nil {
		return false, ErrInvalidChangeTag
	}
	targetType = strings.TrimSpace(targetType)
	if targetType != TagTargetRevision && targetType != TagTargetProposal &&
		targetType != TagTargetAudit {
		return false, ErrInvalidChangeTag
	}
	inserted := false
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var tagExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM change_tag WHERE id=$1)`,
			tagID).Scan(&tagExists); err != nil {
			return err
		}
		if !tagExists {
			return ErrChangeTagNotFound
		}
		pageID, err := s.validateTagTarget(ctx, tx, targetType, targetID)
		if err != nil {
			return err
		}
		if err := s.auth.CheckTx(ctx, tx, actorID, s.wikiID, ActionReview, pageID); err != nil {
			return err
		}
		var revisionID, proposalID, auditID *uuid.UUID
		switch targetType {
		case TagTargetRevision:
			revisionID = &targetID
		case TagTargetProposal:
			proposalID = &targetID
		case TagTargetAudit:
			auditID = &targetID
		}
		command, err := tx.Exec(ctx, `INSERT INTO change_tag_assignment
			(tag_id,revision_id,proposal_id,audit_event_id)
			VALUES ($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
			tagID, revisionID, proposalID, auditID)
		if err != nil {
			return err
		}
		inserted = command.RowsAffected() == 1
		if !inserted {
			return nil
		}
		return s.insertAudit(ctx, tx, actorID, "change_tag.assigned", targetType, targetID,
			map[string]any{"tag_id": tagID, "target_type": targetType, "target_id": targetID})
	})
	return inserted, err
}

func (s *AuditService) validateTagTarget(
	ctx context.Context, tx pgx.Tx, targetType string, targetID uuid.UUID,
) (*uuid.UUID, error) {
	switch targetType {
	case TagTargetRevision:
		var pageID, wikiID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT r.page_id,p.wiki_id FROM revision r
			JOIN page p ON p.id=r.page_id WHERE r.id=$1`, targetID).Scan(&pageID, &wikiID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAuditTargetNotFound
		}
		if err != nil {
			return nil, err
		}
		if wikiID != s.wikiID {
			return nil, ErrAuditTargetNotFound
		}
		return &pageID, nil
	case TagTargetProposal:
		proposal, err := s.repo.GetProposal(ctx, tx, targetID)
		if err != nil {
			return nil, err
		}
		if wikiIDForProposal(ctx, tx, s.repo, proposal) != s.wikiID {
			return nil, ErrProposalNotFound
		}
		if proposal.TargetType == TargetPage {
			return proposal.TargetID, nil
		}
		return nil, nil
	case TagTargetAudit:
		belongs, err := s.auditEventBelongsToWiki(ctx, tx, targetID)
		if err != nil {
			return nil, err
		}
		if !belongs {
			return nil, ErrAuditEventNotFound
		}
		return nil, nil
	default:
		return nil, ErrInvalidChangeTag
	}
}

func (s *AuditService) auditEventBelongsToWiki(
	ctx context.Context, tx pgx.Tx, eventID uuid.UUID,
) (bool, error) {
	visited := map[uuid.UUID]bool{}
	for hops := 0; hops < 8; hops++ {
		if visited[eventID] {
			return false, nil
		}
		visited[eventID] = true
		var aggregateType string
		var aggregateID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT aggregate_type,aggregate_id
			FROM audit_event WHERE id=$1`, eventID).Scan(&aggregateType, &aggregateID)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		var wikiID uuid.UUID
		switch aggregateType {
		case "page":
			err = tx.QueryRow(ctx, `SELECT wiki_id FROM page WHERE id=$1`, aggregateID).Scan(&wikiID)
		case "revision":
			err = tx.QueryRow(ctx, `SELECT p.wiki_id FROM revision r
				JOIN page p ON p.id=r.page_id WHERE r.id=$1`, aggregateID).Scan(&wikiID)
		case "working_document":
			err = tx.QueryRow(ctx, `SELECT p.wiki_id FROM working_document wd
				JOIN page p ON p.id=wd.page_id WHERE wd.id=$1`, aggregateID).Scan(&wikiID)
		case "entity":
			err = tx.QueryRow(ctx, `SELECT wiki_id FROM entity WHERE id=$1`, aggregateID).Scan(&wikiID)
		case "claim":
			err = tx.QueryRow(ctx, `SELECT e.wiki_id FROM claim c
				JOIN entity e ON e.id=c.subject_entity_id WHERE c.id=$1`, aggregateID).Scan(&wikiID)
		case "entity_merge":
			err = tx.QueryRow(ctx, `SELECT e.wiki_id FROM entity_merge em
				JOIN entity e ON e.id=em.source_entity_id WHERE em.id=$1`, aggregateID).Scan(&wikiID)
		case "collection":
			err = tx.QueryRow(
				ctx, `SELECT wiki_id FROM collection WHERE id=$1`,
				aggregateID,
			).Scan(&wikiID)
		case "page_protection":
			err = tx.QueryRow(ctx, `SELECT COALESCE(p.wiki_id,n.wiki_id)
				FROM page_protection pp
				LEFT JOIN page p ON p.id=pp.page_id
				LEFT JOIN namespace n ON n.id=pp.namespace_id
				WHERE pp.id=$1`, aggregateID).Scan(&wikiID)
		case "proposal":
			proposal, proposalErr := s.repo.GetProposal(ctx, tx, aggregateID)
			if proposalErr != nil {
				return false, nil
			}
			return wikiIDForProposal(ctx, tx, s.repo, proposal) == s.wikiID, nil
		case "audit_event":
			eventID = aggregateID
			continue
		case "change_tag":
			// Tags are intentionally global in the design and may be viewed by
			// reviewers in every Wiki.
			return true, nil
		default:
			return false, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return wikiID == s.wikiID, nil
	}
	return false, nil
}

func (s *AuditService) List(
	ctx context.Context, actorID uuid.UUID, filter AuditListFilter,
) (*AuditEventPage, error) {
	if err := s.auth.Check(ctx, actorID, s.wikiID, ActionReview, nil); err != nil {
		return nil, err
	}
	filter.EventType = strings.TrimSpace(filter.EventType)
	filter.AggregateType = strings.TrimSpace(filter.AggregateType)
	filter.TagKey = strings.TrimSpace(filter.TagKey)
	if len(filter.EventType) > 120 || len(filter.AggregateType) > 64 ||
		len(filter.TagKey) > 64 {
		return nil, ErrInvalidChangeTag
	}
	if filter.Limit <= 0 {
		filter.Limit = DefaultAuditPageSize
	}
	if filter.Limit > MaxAuditPageSize {
		filter.Limit = MaxAuditPageSize
	}
	var afterTime *time.Time
	afterID := uuid.Nil
	if filter.Cursor != "" {
		var cursor auditCursor
		raw, err := base64.RawURLEncoding.DecodeString(filter.Cursor)
		if err != nil || json.Unmarshal(raw, &cursor) != nil ||
			cursor.CreatedAt.IsZero() || cursor.ID == uuid.Nil {
			return nil, ErrInvalidAuditCursor
		}
		afterTime = &cursor.CreatedAt
		afterID = cursor.ID
	}
	rows, err := s.repo.q(nil).Query(ctx, `SELECT ae.id,ae.actor_id,a.display_name,
			ae.event_type,ae.aggregate_type,ae.aggregate_id,ae.change_batch_id,
			ae.payload_json,ae.created_at,
			COALESCE((
				SELECT jsonb_agg(jsonb_build_object(
					'id',ct.id,'tag_key',ct.tag_key,'name',ct.name,
					'description',ct.description,'created_at',ct.created_at
				) ORDER BY ct.tag_key,ct.id)
				FROM change_tag_assignment cta
				JOIN change_tag ct ON ct.id=cta.tag_id
				WHERE cta.audit_event_id=ae.id
			),'[]'::jsonb)
		FROM audit_event ae
		JOIN actor a ON a.id=ae.actor_id
		WHERE ($1='' OR ae.event_type=$1)
		  AND ($2='' OR ae.aggregate_type=$2)
		  AND ($3::uuid IS NULL OR ae.change_batch_id=$3)
		  AND ($4='' OR EXISTS (
			SELECT 1 FROM change_tag_assignment cta
			JOIN change_tag ct ON ct.id=cta.tag_id
			WHERE cta.audit_event_id=ae.id AND ct.tag_key=$4
		  ))
		  AND ($5::timestamptz IS NULL OR (ae.created_at,ae.id)<($5,$6))
		  AND (
			(ae.aggregate_type='page' AND EXISTS (
				SELECT 1 FROM page p WHERE p.id=ae.aggregate_id AND p.wiki_id=$7
			))
			OR (ae.aggregate_type='revision' AND EXISTS (
				SELECT 1 FROM revision r JOIN page p ON p.id=r.page_id
				WHERE r.id=ae.aggregate_id AND p.wiki_id=$7
			))
			OR (ae.aggregate_type='working_document' AND EXISTS (
				SELECT 1 FROM working_document wd JOIN page p ON p.id=wd.page_id
				WHERE wd.id=ae.aggregate_id AND p.wiki_id=$7
			))
			OR (ae.aggregate_type='entity' AND EXISTS (
				SELECT 1 FROM entity e WHERE e.id=ae.aggregate_id AND e.wiki_id=$7
			))
			OR (ae.aggregate_type='claim' AND EXISTS (
				SELECT 1 FROM claim c JOIN entity e ON e.id=c.subject_entity_id
				WHERE c.id=ae.aggregate_id AND e.wiki_id=$7
			))
			OR (ae.aggregate_type='entity_merge' AND EXISTS (
				SELECT 1 FROM entity_merge em JOIN entity e ON e.id=em.source_entity_id
				WHERE em.id=ae.aggregate_id AND e.wiki_id=$7
			))
			OR (ae.aggregate_type='collection' AND EXISTS (
				SELECT 1 FROM collection c
				WHERE c.id=ae.aggregate_id AND c.wiki_id=$7
			))
			OR (ae.aggregate_type='page_protection' AND EXISTS (
				SELECT 1 FROM page_protection pp
				LEFT JOIN page p ON p.id=pp.page_id
				LEFT JOIN namespace n ON n.id=pp.namespace_id
				WHERE pp.id=ae.aggregate_id
				  AND COALESCE(p.wiki_id,n.wiki_id)=$7
			))
			OR (ae.aggregate_type='proposal' AND EXISTS (
				SELECT 1 FROM proposal pr
				WHERE pr.id=ae.aggregate_id AND (
					(pr.target_type='wiki' AND pr.target_id=$7)
					OR (pr.target_type='page' AND EXISTS (
						SELECT 1 FROM page p WHERE p.id=pr.target_id AND p.wiki_id=$7
					))
					OR (pr.target_type='entity' AND EXISTS (
						SELECT 1 FROM entity e WHERE e.id=pr.target_id AND e.wiki_id=$7
					))
					OR (pr.target_type='claim' AND EXISTS (
						SELECT 1 FROM claim c JOIN entity e ON e.id=c.subject_entity_id
						WHERE c.id=pr.target_id AND e.wiki_id=$7
					))
					OR (pr.target_type='collection' AND EXISTS (
						SELECT 1 FROM collection c WHERE c.id=pr.target_id AND c.wiki_id=$7
					))
					OR pr.target_type='external_resource'
					OR (pr.target_id IS NULL AND EXISTS (
						SELECT 1 FROM proposal_operation po
						WHERE po.proposal_id=pr.id
						  AND po.target_json->>'wiki_id'=$7::text
					))
				)
			))
			OR ae.aggregate_type IN ('change_tag','audit_event')
		  )
		ORDER BY ae.created_at DESC,ae.id DESC
		LIMIT $8`,
		filter.EventType, filter.AggregateType, filter.ChangeBatchID, filter.TagKey,
		afterTime, afterID, s.wikiID, filter.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := &AuditEventPage{Items: make([]AuditEventView, 0, filter.Limit)}
	for rows.Next() {
		var event AuditEventView
		var tagsJSON []byte
		if err := rows.Scan(
			&event.ID, &event.ActorID, &event.ActorDisplayName, &event.EventType,
			&event.AggregateType, &event.AggregateID, &event.ChangeBatchID,
			&event.Payload, &event.CreatedAt, &tagsJSON,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tagsJSON, &event.Tags); err != nil {
			return nil, err
		}
		result.Items = append(result.Items, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(result.Items) > filter.Limit {
		last := result.Items[filter.Limit-1]
		raw, _ := json.Marshal(auditCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		next := base64.RawURLEncoding.EncodeToString(raw)
		result.NextCursor = &next
		result.Items = result.Items[:filter.Limit]
	}
	return result, nil
}

func (s *AuditService) insertAudit(
	ctx context.Context, tx pgx.Tx, actorID uuid.UUID,
	eventType, aggregateType string, aggregateID uuid.UUID, payload any,
) error {
	eventID, err := s.ids.New()
	if err != nil {
		return err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.repo.InsertAuditWithoutBatch(
		ctx, tx, eventID, actorID, eventType, aggregateType, aggregateID, raw,
	)
}
