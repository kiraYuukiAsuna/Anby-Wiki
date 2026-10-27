package governance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrPageShareNotFound = errors.New("governance: 页面分享不存在")
	ErrInvalidPageShare  = errors.New("governance: 页面分享参数非法")
)

const (
	PageShareRoleViewer = "viewer"
	PageShareRoleEditor = "editor"
)

type PageShare struct {
	ActorID     uuid.UUID `json:"actor_id"`
	Username    string    `json:"username"`
	Email       string    `json:"email"`
	DisplayName string    `json:"display_name"`
	Status      string    `json:"status"`
	RoleKey     string    `json:"role_key"`
	GrantedBy   uuid.UUID `json:"granted_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PageShareList struct {
	Items []PageShare `json:"items"`
}

type PageShareMutationResult struct {
	Share   PageShare `json:"share"`
	Changed bool      `json:"changed"`
}

type PageShareRemovalResult struct {
	ActorID uuid.UUID `json:"actor_id"`
	Removed bool      `json:"removed"`
}

func (s *UserAdminService) ListPageShares(
	ctx context.Context,
	wikiID, actorID, pageID uuid.UUID,
) (*PageShareList, error) {
	if wikiID == uuid.Nil || actorID == uuid.Nil || pageID == uuid.Nil {
		return nil, ErrInvalidPageShare
	}
	if err := s.auth.Check(ctx, actorID, wikiID, ActionManage, nil); err != nil {
		return nil, err
	}
	if err := s.ensurePageInWiki(ctx, s.repo.pool, wikiID, pageID); err != nil {
		return nil, err
	}
	rows, err := s.repo.pool.Query(ctx, `
		SELECT a.id,la.username,la.email,a.display_name,a.status,
		       ps.role_key,ps.granted_by,ps.created_at,ps.updated_at
		FROM page_share ps
		JOIN actor a ON a.id=ps.actor_id
		JOIN local_account la ON la.actor_id=a.id
		WHERE ps.page_id=$1
		ORDER BY lower(la.username),a.id`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PageShare{}
	for rows.Next() {
		share, scanErr := scanPageShare(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, share)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &PageShareList{Items: items}, nil
}

func (s *UserAdminService) UpsertPageShare(
	ctx context.Context,
	wikiID, actorID, pageID uuid.UUID,
	identifier string,
	roleKey string,
) (*PageShareMutationResult, error) {
	roleKey = strings.TrimSpace(roleKey)
	if !validPageShareRole(roleKey) ||
		wikiID == uuid.Nil || actorID == uuid.Nil || pageID == uuid.Nil {
		return nil, ErrInvalidPageShare
	}
	var result *PageShareMutationResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		if err := s.ensurePageInWiki(ctx, tx, wikiID, pageID); err != nil {
			return err
		}
		targetActorID, err := s.findActiveLocalHumanActor(ctx, tx, identifier)
		if err != nil {
			if errors.Is(err, ErrWikiMemberNotFound) {
				return ErrPageShareNotFound
			}
			return err
		}
		var changed bool
		err = tx.QueryRow(ctx, `
			INSERT INTO page_share (page_id,actor_id,role_key,granted_by)
			VALUES ($1,$2,$3,$4)
			ON CONFLICT (page_id,actor_id) DO UPDATE
			SET role_key=EXCLUDED.role_key,
			    granted_by=EXCLUDED.granted_by,
			    updated_at=now()
			WHERE page_share.role_key IS DISTINCT FROM EXCLUDED.role_key
			RETURNING true`, pageID, targetActorID, roleKey, actorID).Scan(&changed)
		if errors.Is(err, pgx.ErrNoRows) {
			changed = false
		} else if err != nil {
			return err
		}
		if changed {
			if err := s.insertPageShareAudit(
				ctx, tx, actorID, pageID, targetActorID,
				"page.share_granted", roleKey,
			); err != nil {
				return err
			}
		}
		share, err := s.getPageShare(ctx, tx, pageID, targetActorID)
		if err != nil {
			return err
		}
		result = &PageShareMutationResult{Share: *share, Changed: changed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) RemovePageShare(
	ctx context.Context,
	wikiID, actorID, pageID, targetActorID uuid.UUID,
) (*PageShareRemovalResult, error) {
	if wikiID == uuid.Nil || actorID == uuid.Nil ||
		pageID == uuid.Nil || targetActorID == uuid.Nil {
		return nil, ErrInvalidPageShare
	}
	var result *PageShareRemovalResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		if err := s.ensurePageInWiki(ctx, tx, wikiID, pageID); err != nil {
			return err
		}
		existing, err := s.getPageShare(ctx, tx, pageID, targetActorID)
		if errors.Is(err, ErrPageShareNotFound) {
			result = &PageShareRemovalResult{ActorID: targetActorID, Removed: false}
			return nil
		}
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `DELETE FROM page_share
			WHERE page_id=$1 AND actor_id=$2`, pageID, targetActorID)
		if err != nil {
			return err
		}
		removed := tag.RowsAffected() == 1
		if removed {
			if err := s.insertPageShareAudit(
				ctx, tx, actorID, pageID, targetActorID,
				"page.share_revoked", existing.RoleKey,
			); err != nil {
				return err
			}
		}
		result = &PageShareRemovalResult{ActorID: targetActorID, Removed: removed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func validPageShareRole(roleKey string) bool {
	return roleKey == PageShareRoleViewer || roleKey == PageShareRoleEditor
}

func (s *UserAdminService) ensurePageInWiki(
	ctx context.Context,
	querier interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	},
	wikiID, pageID uuid.UUID,
) error {
	var exists bool
	err := querier.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM page
		WHERE id=$1 AND wiki_id=$2 AND deleted_at IS NULL
	)`, pageID, wikiID).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrPageShareNotFound
	}
	return nil
}

func (s *UserAdminService) getPageShare(
	ctx context.Context,
	tx pgx.Tx,
	pageID, actorID uuid.UUID,
) (*PageShare, error) {
	share, err := scanPageShare(tx.QueryRow(ctx, `
		SELECT a.id,la.username,la.email,a.display_name,a.status,
		       ps.role_key,ps.granted_by,ps.created_at,ps.updated_at
		FROM page_share ps
		JOIN actor a ON a.id=ps.actor_id
		JOIN local_account la ON la.actor_id=a.id
		WHERE ps.page_id=$1 AND ps.actor_id=$2`, pageID, actorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPageShareNotFound
	}
	if err != nil {
		return nil, err
	}
	return &share, nil
}

func scanPageShare(row pgx.Row) (PageShare, error) {
	var share PageShare
	err := row.Scan(
		&share.ActorID, &share.Username, &share.Email,
		&share.DisplayName, &share.Status, &share.RoleKey,
		&share.GrantedBy, &share.CreatedAt, &share.UpdatedAt,
	)
	return share, err
}

func (s *UserAdminService) insertPageShareAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID, pageID, targetActorID uuid.UUID,
	eventType string,
	roleKey string,
) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"actor_id": targetActorID,
		"role_key": roleKey,
	})
	if err != nil {
		return err
	}
	return s.repo.InsertAuditWithoutBatch(
		ctx, tx, auditID, actorID, eventType, "page", pageID, payload,
	)
}
