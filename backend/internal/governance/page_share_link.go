package governance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrPageShareLinkNotFound = errors.New("governance: 页面分享链接不存在")
	ErrInvalidPageShareLink  = errors.New("governance: 页面分享链接参数非法")
)

const (
	PageShareLinkTypeOnce      = "once"
	PageShareLinkTypePermanent = "permanent"

	pageShareLinkTokenBytes = 32
)

type PageShareLink struct {
	ID        uuid.UUID  `json:"id"`
	PageID    uuid.UUID  `json:"page_id"`
	RoleKey   string     `json:"role_key"`
	LinkType  string     `json:"link_type"`
	CreatedBy uuid.UUID  `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
}

type PageShareLinkCreationResult struct {
	Link  PageShareLink `json:"link"`
	Token string        `json:"token"`
}

type PageShareLinkAcceptanceResult struct {
	WikiID  uuid.UUID `json:"wiki_id"`
	PageID  uuid.UUID `json:"page_id"`
	RoleKey string    `json:"role_key"`
	Share   PageShare `json:"share"`
	Changed bool      `json:"changed"`
}

func (s *UserAdminService) CreatePageShareLink(
	ctx context.Context,
	wikiID, actorID, pageID uuid.UUID,
	roleKey string,
	linkType string,
) (*PageShareLinkCreationResult, error) {
	roleKey = strings.TrimSpace(roleKey)
	linkType = strings.TrimSpace(linkType)
	if !validPageShareRole(roleKey) || !validPageShareLinkType(linkType) ||
		wikiID == uuid.Nil || actorID == uuid.Nil || pageID == uuid.Nil {
		return nil, ErrInvalidPageShareLink
	}
	token, err := newPageShareLinkToken()
	if err != nil {
		return nil, err
	}
	tokenHash := pageShareLinkTokenHash(token)
	var result *PageShareLinkCreationResult
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		if err := s.ensurePageInWiki(ctx, tx, wikiID, pageID); err != nil {
			return err
		}
		linkID, err := s.ids.New()
		if err != nil {
			return err
		}
		link, err := scanPageShareLink(tx.QueryRow(ctx, `
			INSERT INTO page_share_link
				(id,page_id,token_hash,role_key,link_type,created_by)
			VALUES ($1,$2,$3,$4,$5,$6)
			RETURNING id,page_id,role_key,link_type,created_by,created_at,revoked_at`,
			linkID, pageID, tokenHash[:], roleKey, linkType, actorID))
		if err != nil {
			return err
		}
		if err := s.insertPageShareLinkAudit(
			ctx, tx, actorID, pageID, link.ID, "page.share_link_created",
			roleKey, linkType, nil,
		); err != nil {
			return err
		}
		result = &PageShareLinkCreationResult{Link: link, Token: token}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) AcceptPageShareLink(
	ctx context.Context,
	actorID uuid.UUID,
	token string,
) (*PageShareLinkAcceptanceResult, error) {
	token = strings.TrimSpace(token)
	if actorID == uuid.Nil || len(token) < 32 || len(token) > 128 {
		return nil, ErrInvalidPageShareLink
	}
	tokenHash := pageShareLinkTokenHash(token)
	var result *PageShareLinkAcceptanceResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.ensureActiveLocalHumanActor(ctx, tx, actorID); err != nil {
			return err
		}
		var linkID, pageID, wikiID, createdBy uuid.UUID
		var roleKey, linkType string
		var claimedAt *time.Time
		err := tx.QueryRow(ctx, `
			SELECT psl.id,psl.page_id,p.wiki_id,psl.role_key,psl.link_type,
			       psl.created_by,psl.claimed_at
			FROM page_share_link psl
			JOIN page p ON p.id=psl.page_id
			WHERE psl.token_hash=$1
			  AND psl.revoked_at IS NULL
			  AND p.deleted_at IS NULL
			FOR UPDATE OF psl`, tokenHash[:]).Scan(
			&linkID, &pageID, &wikiID, &roleKey, &linkType,
			&createdBy, &claimedAt,
		)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrPageShareLinkNotFound
		}
		if err != nil {
			return err
		}
		if linkType == PageShareLinkTypeOnce && claimedAt != nil {
			return ErrPageShareLinkNotFound
		}
		finalRole, changed, err := s.upsertPageShareGrant(
			ctx, tx, pageID, actorID, roleKey, createdBy,
		)
		if err != nil {
			return err
		}
		if linkType == PageShareLinkTypeOnce {
			if _, err := tx.Exec(ctx, `
				UPDATE page_share_link
				SET claimed_by=$1, claimed_at=now(), updated_at=now()
				WHERE id=$2`, actorID, linkID); err != nil {
				return err
			}
		}
		share, err := s.getPageShare(ctx, tx, pageID, actorID)
		if err != nil {
			return err
		}
		if err := s.insertPageShareLinkAudit(
			ctx, tx, actorID, pageID, linkID, "page.share_link_claimed",
			finalRole, linkType, &actorID,
		); err != nil {
			return err
		}
		result = &PageShareLinkAcceptanceResult{
			WikiID: wikiID, PageID: pageID, RoleKey: finalRole,
			Share: *share, Changed: changed,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) upsertPageShareGrant(
	ctx context.Context,
	tx pgx.Tx,
	pageID, actorID uuid.UUID,
	roleKey string,
	grantedBy uuid.UUID,
) (string, bool, error) {
	var existingRole string
	err := tx.QueryRow(ctx, `SELECT role_key FROM page_share
		WHERE page_id=$1 AND actor_id=$2`, pageID, actorID).Scan(&existingRole)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `INSERT INTO page_share
			(page_id,actor_id,role_key,granted_by)
			VALUES ($1,$2,$3,$4)`, pageID, actorID, roleKey, grantedBy)
		return roleKey, true, err
	}
	if err != nil {
		return "", false, err
	}
	if existingRole == PageShareRoleEditor || existingRole == roleKey {
		return existingRole, false, nil
	}
	_, err = tx.Exec(ctx, `UPDATE page_share
		SET role_key=$1, granted_by=$2, updated_at=now()
		WHERE page_id=$3 AND actor_id=$4`,
		PageShareRoleEditor, grantedBy, pageID, actorID)
	return PageShareRoleEditor, true, err
}

func (s *UserAdminService) ensureActiveLocalHumanActor(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
) error {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM actor a
		JOIN local_account la ON la.actor_id=a.id
		WHERE a.id=$1
		  AND a.actor_type='human'
		  AND a.status='active'
		  AND la.status='active'
	)`, actorID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrInvalidActor
	}
	return nil
}

func validPageShareLinkType(linkType string) bool {
	return linkType == PageShareLinkTypeOnce ||
		linkType == PageShareLinkTypePermanent
}

func newPageShareLinkToken() (string, error) {
	var raw [pageShareLinkTokenBytes]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func pageShareLinkTokenHash(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}

func scanPageShareLink(row pgx.Row) (PageShareLink, error) {
	var link PageShareLink
	err := row.Scan(
		&link.ID, &link.PageID, &link.RoleKey, &link.LinkType,
		&link.CreatedBy, &link.CreatedAt, &link.RevokedAt,
	)
	return link, err
}

func (s *UserAdminService) insertPageShareLinkAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID, pageID, linkID uuid.UUID,
	eventType string,
	roleKey string,
	linkType string,
	claimedBy *uuid.UUID,
) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	payload := map[string]any{
		"link_id":   linkID,
		"role_key":  roleKey,
		"link_type": linkType,
	}
	if claimedBy != nil {
		payload["claimed_by"] = *claimedBy
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return s.repo.InsertAuditWithoutBatch(
		ctx, tx, auditID, actorID, eventType, "page", pageID, encoded,
	)
}
