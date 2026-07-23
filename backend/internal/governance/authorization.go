package governance

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
)

var actionRoles = map[string]map[string]bool{
	ActionCreate:        {"editor": true, "admin": true},
	ActionEdit:          {"editor": true, "admin": true},
	ActionRename:        {"editor": true, "admin": true},
	ActionReview:        {"reviewer": true, "admin": true},
	ActionApply:         {"applier": true, "admin": true},
	ActionBatchRollback: {"applier": true, "admin": true},
	ActionEntityMerge:   {"admin": true},
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

// CheckCreate 对尚不存在的页面按 Namespace + normalized title 叠加保护规则。
func (s *AuthorizationService) CheckCreate(ctx context.Context, actorID, wikiID, namespaceID uuid.UUID, normalizedTitle string) error {
	return s.checkTx(ctx, nil, actorID, wikiID, ActionCreate, nil, &namespaceID, normalizedTitle)
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
		return fmt.Errorf("%w: action=%s", ErrPermissionDenied, action)
	}
	if pageID == nil && (action != ActionCreate || namespaceID == nil || normalizedTitle == "") {
		return nil
	}
	var requiredRole string
	if pageID != nil {
		err = s.q(tx).QueryRow(ctx, `SELECT r.role_key FROM page_protection pp
			JOIN role r ON r.id=pp.required_role_id
			WHERE pp.page_id=$1 AND pp.action_type=$2 AND (pp.expires_at IS NULL OR pp.expires_at>now())
			ORDER BY pp.created_at DESC LIMIT 1`, *pageID, action).Scan(&requiredRole)
	} else {
		err = s.q(tx).QueryRow(ctx, `SELECT r.role_key FROM page_protection pp
			JOIN role r ON r.id=pp.required_role_id
			WHERE pp.page_id IS NULL AND pp.namespace_id=$1 AND pp.normalized_title=$2
				AND pp.action_type='create' AND (pp.expires_at IS NULL OR pp.expires_at>now())
			ORDER BY pp.created_at DESC LIMIT 1`, *namespaceID, normalizedTitle).Scan(&requiredRole)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if roles[requiredRole] || roles["admin"] {
		return nil
	}
	return fmt.Errorf("%w: page protection requires=%s", ErrPermissionDenied, requiredRole)
}

func wikiIDForProposal(ctx context.Context, tx pgx.Tx, repo *Repository, p *Proposal) uuid.UUID {
	if p == nil {
		return uuid.Nil
	}
	if p.TargetID == nil {
		// create_entity has no stable entity ID before Apply. Resolve its wiki from
		// the frozen operation target so ordinary wiki-scoped review roles still work.
		if p.TargetType == TargetEntity {
			operations, err := repo.ListOperations(ctx, tx, p.ID)
			if err == nil {
				for i := range operations {
					op, parseErr := OperationFromRecord(&operations[i])
					if parseErr == nil && op.OperationType == OpCreateEntity && op.Target.WikiID != nil {
						return *op.Target.WikiID
					}
				}
			}
		}
		return uuid.Nil
	}
	var wikiID uuid.UUID
	var err error
	switch p.TargetType {
	case TargetPage:
		err = repo.q(tx).QueryRow(ctx, `SELECT wiki_id FROM page WHERE id=$1`, *p.TargetID).Scan(&wikiID)
	case TargetEntity:
		err = repo.q(tx).QueryRow(ctx, `SELECT wiki_id FROM entity WHERE id=$1`, *p.TargetID).Scan(&wikiID)
	case TargetClaim:
		err = repo.q(tx).QueryRow(ctx, `SELECT e.wiki_id FROM claim c JOIN entity e ON e.id=c.subject_entity_id WHERE c.id=$1`, *p.TargetID).Scan(&wikiID)
	default:
		return uuid.Nil
	}
	if err != nil {
		return uuid.Nil
	}
	return wikiID
}
