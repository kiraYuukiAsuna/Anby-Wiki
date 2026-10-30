package governance

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	authdomain "github.com/anby/wiki/backend/internal/auth"
	"github.com/anby/wiki/backend/internal/platform/db"
)

var ErrPermissionDenied = errors.New("governance: 权限不足")

const (
	ActionCreate        = "create"
	ActionEdit          = "edit"
	ActionRename        = "rename"
	ActionReview        = "review"
	ActionApply         = "apply"
	ActionBatchRollback = "batch_rollback"
	ActionEntityMerge   = "entity_merge"
	ActionManage        = "manage"
)

var actionRoles = map[string]map[string]bool{
	ActionCreate:        {"editor": true, "admin": true},
	ActionEdit:          {"editor": true, "admin": true},
	ActionRename:        {"editor": true, "admin": true},
	ActionReview:        {"reviewer": true, "admin": true},
	ActionApply:         {"applier": true, "admin": true},
	ActionBatchRollback: {"applier": true, "admin": true},
	ActionEntityMerge:   {"admin": true},
	ActionManage:        {"admin": true},
}

type AuthorizationService struct{ pool db.Querier }

func NewAuthorizationService(pool db.Querier) *AuthorizationService {
	return &AuthorizationService{pool: pool}
}

func (s *AuthorizationService) q(tx pgx.Tx) db.Querier {
	if tx != nil {
		return tx
	}
	return s.pool
}

func (s *AuthorizationService) Check(ctx context.Context, actorID, wikiID uuid.UUID, action string, pageID *uuid.UUID) error {
	return s.CheckTx(ctx, nil, actorID, wikiID, action, pageID)
}

// CheckAccountManage authorizes management of the actor's own Wiki, independent
// of the Wiki selected for collaboration.
func (s *AuthorizationService) CheckAccountManage(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error) {
	var wikiID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT id FROM wiki_site WHERE owner_actor_id=$1`, actorID).Scan(&wikiID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrPermissionDenied
	}
	if err != nil {
		return uuid.Nil, err
	}
	if err := s.Check(ctx, actorID, wikiID, ActionManage, nil); err != nil {
		return uuid.Nil, err
	}
	return wikiID, nil
}

func (s *AuthorizationService) CheckReadPage(ctx context.Context, actorID, wikiID, pageID uuid.UUID) error {
	var actorType, status string
	if err := s.pool.QueryRow(ctx, `SELECT actor_type,status FROM actor WHERE id=$1`, actorID).Scan(&actorType, &status); err != nil {
		return ErrInvalidActor
	}
	if status != "active" {
		return ErrInvalidActor
	}
	if actorType == "system" {
		return nil
	}
	if actorType != "human" && actorType != "ai" && actorType != "import" {
		return ErrPermissionDenied
	}
	hasWikiAccess, err := s.HasWikiAccess(ctx, actorID, wikiID)
	if err != nil {
		return err
	}
	if hasWikiAccess {
		return nil
	}
	if actorType == authdomain.ActorTypeAI || actorType == authdomain.ActorTypeImport {
		return ErrPermissionDenied
	}
	shared, err := s.hasPageShareRole(
		ctx, nil, actorID, wikiID, pageID,
		PageShareRoleViewer, PageShareRoleEditor,
	)
	if err != nil {
		return err
	}
	if shared {
		return nil
	}
	return ErrPermissionDenied
}

func (s *AuthorizationService) HasWikiAccess(ctx context.Context, actorID, wikiID uuid.UUID) (bool, error) {
	var allowed bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM actor a
		LEFT JOIN local_account la ON la.actor_id=a.id
		WHERE a.id=$1 AND a.status='active'
		  AND (la.id IS NULL OR la.status='active')
		  AND (
		    a.actor_type='system'
		    OR (a.actor_type IN ('ai','import') AND a.wiki_id=$2
		      AND starts_with(a.external_key,$3)
		      AND EXISTS (SELECT 1 FROM local_account owner_account
		        JOIN actor owner ON owner.id=owner_account.actor_id
		        JOIN actor_role ar ON ar.actor_id=owner.id AND ar.wiki_id=$2
		        JOIN role role ON role.id=ar.role_id AND role.role_key='admin'
		        WHERE owner_account.id=a.user_id AND owner_account.status='active'
		          AND owner.status='active' AND owner.actor_type='human'))
		    OR (a.actor_type='human' AND EXISTS (
		      SELECT 1 FROM actor_role ar
		      WHERE ar.actor_id=a.id AND ar.wiki_id=$2
		    ))
		  )
	)`, actorID, wikiID, authdomain.MachineActorKeyPrefix).Scan(&allowed)
	return allowed, err
}

func (s *AuthorizationService) SharedPageIDs(ctx context.Context, actorID, wikiID uuid.UUID) ([]uuid.UUID, error) {
	hasWikiAccess, err := s.HasWikiAccess(ctx, actorID, wikiID)
	if err != nil {
		return nil, err
	}
	if hasWikiAccess {
		return nil, nil
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ps.page_id
		FROM page_share ps
		JOIN page p ON p.id=ps.page_id
		WHERE ps.actor_id=$1 AND p.wiki_id=$2 AND p.deleted_at IS NULL
		ORDER BY p.updated_at DESC,p.id DESC`, actorID, wikiID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// CheckCreate 对尚不存在的页面按 Namespace + normalized title 叠加保护规则。
func (s *AuthorizationService) CheckCreate(ctx context.Context, actorID, wikiID, namespaceID uuid.UUID, normalizedTitle string) error {
	return s.checkTx(ctx, nil, actorID, wikiID, ActionCreate, nil, &namespaceID, normalizedTitle)
}

// CheckCreateTx is the transaction-aware form used by Proposal Apply.
func (s *AuthorizationService) CheckCreateTx(
	ctx context.Context,
	tx pgx.Tx,
	actorID, wikiID, namespaceID uuid.UUID,
	normalizedTitle string,
) error {
	return s.checkTx(
		ctx, tx, actorID, wikiID, ActionCreate, nil, &namespaceID, normalizedTitle,
	)
}

// CheckTx 执行基础 Role 与 PageProtection 叠加授权。system 为运维恢复通道；
// ai/import/anonymous 无论被误授何种 Role 都不能直接编辑、审核或 Apply。
func (s *AuthorizationService) CheckTx(ctx context.Context, tx pgx.Tx, actorID, wikiID uuid.UUID, action string, pageID *uuid.UUID) error {
	return s.checkTx(ctx, tx, actorID, wikiID, action, pageID, nil, "")
}

func (s *AuthorizationService) checkTx(ctx context.Context, tx pgx.Tx, actorID, wikiID uuid.UUID, action string, pageID, namespaceID *uuid.UUID, normalizedTitle string) error {
	allowedRoles, ok := actionRoles[action]
	if !ok {
		return fmt.Errorf("%w: unknown action=%s", ErrPermissionDenied, action)
	}
	var actorType, status string
	if err := s.q(tx).QueryRow(ctx, `SELECT actor_type,status FROM actor WHERE id=$1`, actorID).Scan(&actorType, &status); err != nil {
		return ErrInvalidActor
	}
	if status != "active" {
		return ErrInvalidActor
	}
	if actorType == "system" {
		return nil
	}
	if actorType == "ai" || actorType == "import" || actorType == "anonymous" {
		return ErrPermissionDenied
	}
	rows, err := s.q(tx).Query(ctx, `SELECT r.role_key FROM actor_role ar JOIN role r ON r.id=ar.role_id
		WHERE ar.actor_id=$1 AND ar.wiki_id=$2`, actorID, wikiID)
	if err != nil {
		return err
	}
	roles := map[string]bool{}
	for rows.Next() {
		var role string
		if err := rows.Scan(&role); err != nil {
			rows.Close()
			return err
		}
		roles[role] = true
	}
	rows.Close()
	baseAllowed := false
	for role := range roles {
		baseAllowed = baseAllowed || allowedRoles[role]
	}
	if !baseAllowed {
		if action == ActionEdit && pageID != nil {
			shared, err := s.hasPageShareRole(
				ctx, tx, actorID, wikiID, *pageID, PageShareRoleEditor,
			)
			if err != nil {
				return err
			}
			if shared {
				protected, err := s.hasActivePageProtection(ctx, tx, *pageID, action)
				if err != nil {
					return err
				}
				if protected {
					return fmt.Errorf("%w: page protection requires wiki role", ErrPermissionDenied)
				}
				return nil
			}
		}
		return fmt.Errorf("%w: action=%s", ErrPermissionDenied, action)
	}
	if pageID == nil && (action != ActionCreate || namespaceID == nil || normalizedTitle == "") {
		return nil
	}
	if roles["admin"] {
		return nil
	}
	var protectionRows pgx.Rows
	if pageID != nil {
		protectionRows, err = s.q(tx).Query(ctx, `SELECT DISTINCT r.role_key
			FROM page_protection pp
			JOIN role r ON r.id=pp.required_role_id
			WHERE pp.page_id=$1 AND pp.action_type=$2
			  AND pp.revoked_at IS NULL
			  AND (pp.expires_at IS NULL OR pp.expires_at>now())
			ORDER BY r.role_key`, *pageID, action)
	} else {
		protectionRows, err = s.q(tx).Query(ctx, `SELECT DISTINCT r.role_key
			FROM page_protection pp
			JOIN role r ON r.id=pp.required_role_id
			WHERE pp.page_id IS NULL AND pp.namespace_id=$1 AND pp.normalized_title=$2
				AND pp.action_type='create' AND pp.revoked_at IS NULL
				AND (pp.expires_at IS NULL OR pp.expires_at>now())
			ORDER BY r.role_key`, *namespaceID, normalizedTitle)
	}
	if err != nil {
		return err
	}
	defer protectionRows.Close()
	for protectionRows.Next() {
		var requiredRole string
		if err := protectionRows.Scan(&requiredRole); err != nil {
			return err
		}
		if !roles[requiredRole] {
			return fmt.Errorf(
				"%w: page protection requires=%s",
				ErrPermissionDenied, requiredRole,
			)
		}
	}
	return protectionRows.Err()
}

func (s *AuthorizationService) hasPageShareRole(
	ctx context.Context,
	tx pgx.Tx,
	actorID, wikiID, pageID uuid.UUID,
	roleKeys ...string,
) (bool, error) {
	if len(roleKeys) == 0 {
		return false, nil
	}
	args := []any{actorID, wikiID, pageID}
	placeholders := ""
	for index, roleKey := range roleKeys {
		if index > 0 {
			placeholders += ","
		}
		args = append(args, roleKey)
		placeholders += fmt.Sprintf("$%d", len(args))
	}
	var allowed bool
	err := s.q(tx).QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM page_share ps
		JOIN page p ON p.id=ps.page_id
		WHERE ps.actor_id=$1 AND p.wiki_id=$2 AND ps.page_id=$3
		  AND p.deleted_at IS NULL
		  AND ps.role_key IN (`+placeholders+`)
	)`, args...).Scan(&allowed)
	return allowed, err
}

func (s *AuthorizationService) hasActivePageProtection(
	ctx context.Context,
	tx pgx.Tx,
	pageID uuid.UUID,
	action string,
) (bool, error) {
	var protected bool
	err := s.q(tx).QueryRow(ctx, `SELECT EXISTS(
		SELECT 1
		FROM page_protection pp
		WHERE pp.page_id=$1 AND pp.action_type=$2
		  AND pp.revoked_at IS NULL
		  AND (pp.expires_at IS NULL OR pp.expires_at>now())
	)`, pageID, action).Scan(&protected)
	return protected, err
}

func wikiIDForProposal(ctx context.Context, tx pgx.Tx, repo *Repository, p *Proposal) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	return p.WikiID
}
