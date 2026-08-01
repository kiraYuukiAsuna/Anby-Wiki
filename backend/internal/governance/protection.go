package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/anby/wiki/backend/internal/page"
	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var (
	ErrProtectionNotFound = errors.New("governance: PageProtection 不存在")
	ErrInvalidProtection  = errors.New("governance: PageProtection 参数非法")
)

var validProtectionActions = map[string]bool{
	ActionCreate: true, ActionEdit: true, ActionRename: true,
	ActionReview: true, ActionApply: true, ActionBatchRollback: true,
}

type PageProtection struct {
	ID               uuid.UUID  `json:"id"`
	PageID           *uuid.UUID `json:"page_id"`
	PageTitle        *string    `json:"page_title"`
	NamespaceID      *uuid.UUID `json:"namespace_id"`
	NamespaceKey     *string    `json:"namespace_key"`
	NormalizedTitle  *string    `json:"normalized_title"`
	ActionType       string     `json:"action_type"`
	RequiredRoleID   uuid.UUID  `json:"required_role_id"`
	RequiredRoleKey  string     `json:"required_role_key"`
	RequiredRoleName string     `json:"required_role_name"`
	ExpiresAt        *time.Time `json:"expires_at"`
	CreatedBy        uuid.UUID  `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	RevokedAt        *time.Time `json:"revoked_at"`
	RevokedBy        *uuid.UUID `json:"revoked_by"`
}

type RoleSummary struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
}

type CreatePageProtectionParams struct {
	WikiID          uuid.UUID
	PageID          *uuid.UUID
	NamespaceID     *uuid.UUID
	NamespaceKey    string
	NormalizedTitle string
	ActionType      string
	RequiredRoleKey string
	ExpiresAt       *time.Time
	ActorID         uuid.UUID
}

type ProtectionService struct {
	repo *Repository
	auth *AuthorizationService
	txm  *db.TxManager
	ids  *id.Generator
}

func NewProtectionService(
	repo *Repository,
	auth *AuthorizationService,
	txm *db.TxManager,
	ids *id.Generator,
) *ProtectionService {
	return &ProtectionService{repo: repo, auth: auth, txm: txm, ids: ids}
}

func (s *ProtectionService) List(
	ctx context.Context,
	wikiID, actorID uuid.UUID,
	pageID *uuid.UUID,
	includeExpired bool,
) ([]PageProtection, error) {
	if err := s.auth.Check(
		ctx, actorID, wikiID, ActionManage, nil,
	); err != nil {
		return nil, err
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT
		pp.id,pp.page_id,p.display_title,pp.namespace_id,n.namespace_key,
		pp.normalized_title,pp.action_type,pp.required_role_id,
		r.role_key,r.name,pp.expires_at,pp.created_by,pp.created_at,
		pp.revoked_at,pp.revoked_by
		FROM page_protection pp
		JOIN role r ON r.id=pp.required_role_id
		LEFT JOIN page p ON p.id=pp.page_id
		LEFT JOIN namespace n ON n.id=pp.namespace_id
		WHERE COALESCE(p.wiki_id,n.wiki_id)=$1
		  AND ($2::uuid IS NULL OR pp.page_id=$2)
		  AND ($3 OR (
			pp.revoked_at IS NULL
			AND (pp.expires_at IS NULL OR pp.expires_at>now())
		  ))
		ORDER BY (pp.revoked_at IS NOT NULL),
			(pp.expires_at IS NOT NULL AND pp.expires_at<=now()),
			pp.created_at DESC,pp.id DESC
		LIMIT 500`, wikiID, pageID, includeExpired)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PageProtection{}
	for rows.Next() {
		var item PageProtection
		if err := rows.Scan(
			&item.ID, &item.PageID, &item.PageTitle,
			&item.NamespaceID, &item.NamespaceKey, &item.NormalizedTitle,
			&item.ActionType, &item.RequiredRoleID,
			&item.RequiredRoleKey, &item.RequiredRoleName,
			&item.ExpiresAt, &item.CreatedBy, &item.CreatedAt,
			&item.RevokedAt, &item.RevokedBy,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *ProtectionService) ListRoles(
	ctx context.Context,
	wikiID, actorID uuid.UUID,
) ([]RoleSummary, error) {
	if err := s.auth.Check(
		ctx, actorID, wikiID, ActionManage, nil,
	); err != nil {
		return nil, err
	}
	rows, err := s.repo.pool.Query(ctx, `SELECT id,role_key,name,description
		FROM role ORDER BY role_key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	roles := []RoleSummary{}
	for rows.Next() {
		var role RoleSummary
		if err := rows.Scan(
			&role.ID, &role.Key, &role.Name, &role.Description,
		); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	return roles, rows.Err()
}

func (s *ProtectionService) Create(
	ctx context.Context,
	params CreatePageProtectionParams,
) (*PageProtection, error) {
	params.ActionType = strings.TrimSpace(params.ActionType)
	params.RequiredRoleKey = strings.TrimSpace(params.RequiredRoleKey)
	params.NamespaceKey = strings.TrimSpace(params.NamespaceKey)
	if params.WikiID == uuid.Nil || params.ActorID == uuid.Nil ||
		!validProtectionActions[params.ActionType] ||
		params.RequiredRoleKey == "" {
		return nil, ErrInvalidProtection
	}
	pageScope := params.PageID != nil
	titleScope := (params.NamespaceID != nil || params.NamespaceKey != "") &&
		strings.TrimSpace(params.NormalizedTitle) != ""
	if pageScope == titleScope ||
		(params.NamespaceID != nil && params.NamespaceKey != "") {
		return nil, ErrInvalidProtection
	}
	if pageScope && params.ActionType == ActionCreate {
		return nil, fmt.Errorf(
			"%w: create 保护必须使用 Namespace + title 范围",
			ErrInvalidProtection,
		)
	}
	if titleScope && params.ActionType != ActionCreate {
		return nil, fmt.Errorf(
			"%w: 标题范围只支持 create 保护", ErrInvalidProtection,
		)
	}
	if params.ExpiresAt != nil &&
		!params.ExpiresAt.After(time.Now().UTC()) {
		return nil, fmt.Errorf(
			"%w: expires_at 必须晚于当前时间", ErrInvalidProtection,
		)
	}
	if titleScope {
		normalized, err := page.NormalizeTitle(params.NormalizedTitle)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidProtection, err)
		}
		params.NormalizedTitle = normalized
	}

	var created *PageProtection
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(
			ctx, tx, params.ActorID, params.WikiID, ActionManage, nil,
		); err != nil {
			return err
		}
		if params.PageID != nil {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(
				SELECT 1 FROM page
				WHERE id=$1 AND wiki_id=$2 AND deleted_at IS NULL
			)`, *params.PageID, params.WikiID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return page.ErrPageNotFound
			}
		} else if params.NamespaceKey != "" {
			var namespaceID uuid.UUID
			if err := tx.QueryRow(ctx, `SELECT id
				FROM namespace
				WHERE namespace_key=$1 AND wiki_id=$2`,
				params.NamespaceKey, params.WikiID,
			).Scan(&namespaceID); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return page.ErrNamespaceNotFound
				}
				return err
			}
			params.NamespaceID = &namespaceID
		} else {
			var exists bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(
				SELECT 1 FROM namespace WHERE id=$1 AND wiki_id=$2
			)`, *params.NamespaceID, params.WikiID).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return page.ErrNamespaceNotFound
			}
		}

		var role RoleSummary
		if err := tx.QueryRow(ctx, `SELECT id,role_key,name,description
			FROM role WHERE role_key=$1`,
			params.RequiredRoleKey,
		).Scan(&role.ID, &role.Key, &role.Name, &role.Description); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf(
					"%w: role=%s", ErrInvalidProtection,
					params.RequiredRoleKey,
				)
			}
			return err
		}
		protectionID, err := s.ids.New()
		if err != nil {
			return err
		}
		item := &PageProtection{
			ID: protectionID, PageID: params.PageID,
			NamespaceID: params.NamespaceID, ActionType: params.ActionType,
			RequiredRoleID: role.ID, RequiredRoleKey: role.Key,
			RequiredRoleName: role.Name, ExpiresAt: params.ExpiresAt,
			CreatedBy: params.ActorID,
		}
		if titleScope {
			title := params.NormalizedTitle
			item.NormalizedTitle = &title
		}
		if err := tx.QueryRow(ctx, `INSERT INTO page_protection
			(id,page_id,namespace_id,normalized_title,action_type,
			 required_role_id,expires_at,created_by)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			RETURNING created_at`,
			item.ID, item.PageID, item.NamespaceID, item.NormalizedTitle,
			item.ActionType, item.RequiredRoleID, item.ExpiresAt,
			item.CreatedBy,
		).Scan(&item.CreatedAt); err != nil {
			return err
		}
		if item.PageID != nil {
			_ = tx.QueryRow(ctx,
				`SELECT display_title FROM page WHERE id=$1`,
				*item.PageID,
			).Scan(&item.PageTitle)
		}
		if item.NamespaceID != nil {
			_ = tx.QueryRow(ctx,
				`SELECT namespace_key FROM namespace WHERE id=$1`,
				*item.NamespaceID,
			).Scan(&item.NamespaceKey)
		}
		if err := s.insertProtectionAudit(
			ctx, tx, params.ActorID, "page_protection.created", item,
		); err != nil {
			return err
		}
		created = item
		return nil
	})
	return created, err
}

func (s *ProtectionService) Delete(
	ctx context.Context,
	wikiID, protectionID, actorID uuid.UUID,
) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		item, itemWikiID, err := s.getForUpdate(
			ctx, tx, protectionID,
		)
		if err != nil {
			return err
		}
		if itemWikiID != wikiID {
			return ErrProtectionNotFound
		}
		if err := s.auth.CheckTx(
			ctx, tx, actorID, wikiID, ActionManage, nil,
		); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `UPDATE page_protection
			SET revoked_at=now(),revoked_by=$2
			WHERE id=$1 AND revoked_at IS NULL
			RETURNING revoked_at,revoked_by`,
			protectionID, actorID,
		).Scan(&item.RevokedAt, &item.RevokedBy); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProtectionNotFound
			}
			return err
		}
		return s.insertProtectionAudit(
			ctx, tx, actorID, "page_protection.deleted", item,
		)
	})
}

func (s *ProtectionService) getForUpdate(
	ctx context.Context,
	tx pgx.Tx,
	id uuid.UUID,
) (*PageProtection, uuid.UUID, error) {
	var item PageProtection
	var wikiID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT
		pp.id,pp.page_id,p.display_title,pp.namespace_id,n.namespace_key,
		pp.normalized_title,pp.action_type,pp.required_role_id,
		r.role_key,r.name,pp.expires_at,pp.created_by,pp.created_at,
		pp.revoked_at,pp.revoked_by,
		COALESCE(p.wiki_id,n.wiki_id)
		FROM page_protection pp
		JOIN role r ON r.id=pp.required_role_id
		LEFT JOIN page p ON p.id=pp.page_id
		LEFT JOIN namespace n ON n.id=pp.namespace_id
		WHERE pp.id=$1 FOR UPDATE OF pp`, id).Scan(
		&item.ID, &item.PageID, &item.PageTitle,
		&item.NamespaceID, &item.NamespaceKey, &item.NormalizedTitle,
		&item.ActionType, &item.RequiredRoleID,
		&item.RequiredRoleKey, &item.RequiredRoleName,
		&item.ExpiresAt, &item.CreatedBy, &item.CreatedAt,
		&item.RevokedAt, &item.RevokedBy, &wikiID,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, uuid.Nil, ErrProtectionNotFound
	}
	return &item, wikiID, err
}

func (s *ProtectionService) insertProtectionAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	eventType string,
	item *PageProtection,
) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	return s.repo.InsertAuditWithoutBatch(
		ctx, tx, auditID, actorID, eventType,
		"page_protection", item.ID, payload,
	)
}
