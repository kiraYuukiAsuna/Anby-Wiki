package page

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const OutboxEventTranslationRequested = "page.translation_requested"

var ErrInvalidTranslation = errors.New("page: 仅支持已发布的中文或英文页面翻译")

type Translation struct {
	SourcePageID     uuid.UUID  `json:"source_page_id"`
	SourceRevisionID uuid.UUID  `json:"source_revision_id"`
	TargetLanguage   string     `json:"target_language"`
	TargetPageID     *uuid.UUID `json:"target_page_id"`
	RequestedBy      uuid.UUID  `json:"-"`
	EventID          uuid.UUID  `json:"-"`
	Status           string     `json:"status"`
}

func NormalizeLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "zh" || strings.HasPrefix(language, "zh-") {
		return "zh-Hans"
	}
	if language == "en" || strings.HasPrefix(language, "en-") {
		return "en"
	}
	return ""
}

func (s *Service) Translation(ctx context.Context, pageID uuid.UUID) (*Translation, error) {
	t, err := s.translation(ctx, nil, pageID)
	if err != nil || t == nil {
		return t, err
	}
	missing, err := s.translationPartnerDeleted(ctx, nil, t, pageID)
	if err != nil {
		return nil, err
	}
	if missing {
		return nil, nil
	}
	return t, nil
}
func (s *Service) translation(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) (*Translation, error) {
	var t Translation
	err := s.repo.q(tx).QueryRow(ctx, `SELECT source_page_id,source_revision_id,target_language,target_page_id,requested_by,event_id,CASE WHEN e.status='dead' AND t.status='pending' THEN 'failed' ELSE t.status END
 FROM page_translation t JOIN outbox_event e ON e.id=t.event_id WHERE source_page_id=$1 OR target_page_id=$1`, pageID).Scan(
		&t.SourcePageID, &t.SourceRevisionID, &t.TargetLanguage, &t.TargetPageID, &t.RequestedBy, &t.EventID, &t.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &t, err
}

func (s *Service) translationPartnerDeleted(ctx context.Context, tx pgx.Tx, t *Translation, pageID uuid.UUID) (bool, error) {
	if t.TargetPageID == nil {
		return false, nil
	}
	other := t.SourcePageID
	if other == pageID {
		other = *t.TargetPageID
	}
	p, err := s.repo.GetPageByID(ctx, tx, other)
	if err != nil {
		return false, err
	}
	return p.DeletedAt != nil, nil
}

// CancelBatchTranslationsInTx runs after rollback has locked/restored the affected pages.
// Removing the active request also invalidates a model result already in flight.
func (s *Service) CancelBatchTranslationsInTx(ctx context.Context, tx pgx.Tx, batchID uuid.UUID) error {
	_, err := tx.Exec(ctx, `DELETE FROM page_translation t USING revision r
 WHERE t.source_revision_id=r.id AND r.change_batch_id=$1 AND t.target_page_id IS NULL`, batchID)
	return err
}

func (s *Service) RequestTranslation(ctx context.Context, pageID, actorID uuid.UUID) (*Translation, error) {
	var result *Translation
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		p, err := s.repo.GetPageByIDForUpdate(ctx, tx, pageID)
		if err != nil {
			return err
		}
		if err = s.repo.CheckWriteActor(ctx, tx, actorID); err != nil {
			return err
		}
		result, err = s.requestTranslationInTx(ctx, tx, p, actorID)
		return err
	})
	return result, err
}

func (s *Service) requestTranslationInTx(ctx context.Context, tx pgx.Tx, p *Page, actorID uuid.UUID) (*Translation, error) {
	language := NormalizeLanguage(p.Language)
	if language == "" || p.CurrentRevisionID == nil || p.DeletedAt != nil {
		return nil, ErrInvalidTranslation
	}
	existing, err := s.translation(ctx, tx, p.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		missing, err := s.translationPartnerDeleted(ctx, tx, existing, p.ID)
		if err != nil {
			return nil, err
		}
		if missing {
			if _, err := tx.Exec(ctx, `DELETE FROM page_translation WHERE source_page_id=$1`, existing.SourcePageID); err != nil {
				return nil, err
			}
			existing = nil
		}
	}
	if existing != nil && (existing.Status != "failed" || existing.SourcePageID != p.ID) {
		return existing, nil
	}
	target := "en"
	if language == "en" {
		target = "zh-Hans"
	}
	eventID, err := s.ids.New()
	if err != nil {
		return nil, err
	}
	payload, _ := json.Marshal(map[string]any{"schema_version": 1, "page_id": p.ID, "source_revision_id": p.CurrentRevisionID, "target_language": target})
	if err = s.repo.InsertOutboxEvent(ctx, tx, &OutboxEvent{ID: eventID, AggregateType: AggregateTypePage, AggregateID: p.ID, EventType: OutboxEventTranslationRequested, Payload: payload}); err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO page_translation(source_page_id,source_revision_id,target_language,requested_by,event_id,status)
 VALUES($1,$2,$3,$4,$5,'pending') ON CONFLICT(source_page_id) DO UPDATE SET source_revision_id=$2,requested_by=$4,event_id=$5,status='pending'`, p.ID, *p.CurrentRevisionID, target, actorID, eventID)
	if err != nil {
		return nil, err
	}
	return s.translation(ctx, tx, p.ID)
}

// CompleteTranslation publishes and links the new page in one transaction. Retries cannot overwrite an existing translation.
func (s *Service) CompleteTranslation(ctx context.Context, t Translation, title string, document json.RawMessage, authorize func(context.Context, pgx.Tx, *Page, string) error) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		source, err := s.repo.GetPageByIDForUpdate(ctx, tx, t.SourcePageID)
		if err != nil {
			return err
		}
		current, err := s.translation(ctx, tx, source.ID)
		if err != nil {
			return err
		}
		if current == nil || current.EventID != t.EventID || current.TargetPageID != nil {
			return nil
		}
		if source.DeletedAt != nil {
			return ErrPageNotFound
		}
		normalized, _, err := normalizePair(title)
		if err != nil {
			return err
		}
		if err = authorize(ctx, tx, source, normalized); err != nil {
			return err
		}
		target, err := s.CreatePageInTx(ctx, tx, CreatePageParams{WikiID: source.WikiID, NamespaceID: source.NamespaceID, Title: title, Language: t.TargetLanguage, ContentModel: source.ContentModel, ActorID: t.RequestedBy}, nil)
		if err != nil {
			return err
		}
		if _, err = s.PublishInTx(ctx, tx, PublishParams{PageID: target.ID, ActorID: t.RequestedBy, AST: document, Summary: "Translated from revision " + t.SourceRevisionID.String()}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE page_translation SET target_page_id=$2,status='done' WHERE source_page_id=$1`, source.ID, target.ID)
		return err
	})
}

func (s *Service) FailTranslation(ctx context.Context, pageID, eventID uuid.UUID) error {
	_, err := s.repo.pool.Exec(ctx, `UPDATE page_translation SET status='failed' WHERE source_page_id=$1 AND event_id=$2 AND target_page_id IS NULL`, pageID, eventID)
	return err
}
