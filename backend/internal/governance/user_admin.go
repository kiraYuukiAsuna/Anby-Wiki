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

	"github.com/anby/wiki/backend/internal/platform/db"
	"github.com/anby/wiki/backend/internal/platform/id"
)

var (
	ErrAdminUserNotFound      = errors.New("governance: 用户不存在")
	ErrAdminRoleNotFound      = errors.New("governance: 角色不存在")
	ErrInvalidAdminUserFilter = errors.New("governance: 用户管理参数非法")
	ErrLastAdminRole          = errors.New("governance: 不能撤销最后一个管理员角色")
)

type AdminUser struct {
	ActorID     uuid.UUID     `json:"actor_id"`
	Username    string        `json:"username"`
	Email       string        `json:"email"`
	DisplayName string        `json:"display_name"`
	Status      string        `json:"status"`
	Roles       []RoleSummary `json:"roles"`
	CreatedAt   time.Time     `json:"created_at"`
}

type AdminUserList struct {
	Items []AdminUser `json:"items"`
}

type AdminRoleMutationResult struct {
	User    AdminUser `json:"user"`
	Changed bool      `json:"changed"`
}

type AdminUserListFilter struct {
	Search          string
	IncludeDisabled bool
}

type UserAdminService struct {
	repo *Repository
	auth *AuthorizationService
	txm  *db.TxManager
	ids  *id.Generator
}

func NewUserAdminService(
	repo *Repository,
	auth *AuthorizationService,
	txm *db.TxManager,
	ids *id.Generator,
) *UserAdminService {
	return &UserAdminService{repo: repo, auth: auth, txm: txm, ids: ids}
}

func (s *UserAdminService) List(
	ctx context.Context,
	wikiID, actorID uuid.UUID,
	filter AdminUserListFilter,
) (*AdminUserList, error) {
	if err := s.auth.Check(ctx, actorID, wikiID, ActionManage, nil); err != nil {
		return nil, err
	}
	search := strings.ToLower(strings.TrimSpace(filter.Search))
	if len([]rune(search)) > 128 {
		return nil, ErrInvalidAdminUserFilter
	}
	rows, err := s.repo.pool.Query(ctx, `
		SELECT a.id,la.username,la.email,a.display_name,a.status,a.created_at,
		       COALESCE(json_agg(json_build_object(
		           'id', r.id,
		           'key', r.role_key,
		           'name', r.name,
		           'description', r.description
		       ) ORDER BY r.role_key) FILTER (WHERE r.id IS NOT NULL), '[]'::json)
		FROM actor a
		JOIN local_account la ON la.actor_id=a.id
		LEFT JOIN actor_role ar ON ar.actor_id=a.id AND ar.wiki_id=$1
		LEFT JOIN role r ON r.id=ar.role_id
		WHERE a.actor_type='human'
		  AND ($2 OR a.status='active')
		  AND (
		    $3=''
		    OR lower(la.username) LIKE '%' || $3 || '%'
		    OR lower(la.email) LIKE '%' || $3 || '%'
		    OR lower(a.display_name) LIKE '%' || $3 || '%'
		    OR a.id::text=$3
		  )
		GROUP BY a.id,la.username,la.email,a.display_name,a.status,a.created_at
		ORDER BY lower(la.username),a.id
		LIMIT 500`, wikiID, filter.IncludeDisabled, search)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AdminUser{}
	for rows.Next() {
		user, err := scanAdminUser(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, user)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &AdminUserList{Items: items}, nil
}

func (s *UserAdminService) GrantRole(
	ctx context.Context,
	wikiID, actorID, targetActorID uuid.UUID,
	roleKey string,
) (*AdminRoleMutationResult, error) {
	roleKey = strings.TrimSpace(roleKey)
	if wikiID == uuid.Nil || actorID == uuid.Nil ||
		targetActorID == uuid.Nil || roleKey == "" {
		return nil, ErrInvalidAdminUserFilter
	}
	var result *AdminRoleMutationResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		if err := s.lockAdminRoleMutation(ctx, tx, roleKey); err != nil {
			return err
		}
		if _, err := s.ensureLocalHumanUser(ctx, tx, targetActorID); err != nil {
			return err
		}
		role, err := s.roleByKey(ctx, tx, roleKey)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO actor_role (actor_id,role_id,wiki_id)
			VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
			targetActorID, role.ID, wikiID)
		if err != nil {
			return err
		}
		changed := tag.RowsAffected() == 1
		if changed {
			if err := s.insertRoleAudit(
				ctx, tx, actorID, targetActorID, "actor.role_granted", role,
			); err != nil {
				return err
			}
		}
		user, err := s.getUser(ctx, tx, wikiID, targetActorID)
		if err != nil {
			return err
		}
		result = &AdminRoleMutationResult{User: *user, Changed: changed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) RevokeRole(
	ctx context.Context,
	wikiID, actorID, targetActorID uuid.UUID,
	roleKey string,
) (*AdminRoleMutationResult, error) {
	roleKey = strings.TrimSpace(roleKey)
	if wikiID == uuid.Nil || actorID == uuid.Nil ||
		targetActorID == uuid.Nil || roleKey == "" {
		return nil, ErrInvalidAdminUserFilter
	}
	var result *AdminRoleMutationResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		if err := s.lockAdminRoleMutation(ctx, tx, roleKey); err != nil {
			return err
		}
		targetStatus, err := s.ensureLocalHumanUser(ctx, tx, targetActorID)
		if err != nil {
			return err
		}
		role, err := s.roleByKey(ctx, tx, roleKey)
		if err != nil {
			return err
		}
		if role.Key == "admin" && targetStatus == "active" {
			hasRole, err := s.hasRole(ctx, tx, wikiID, targetActorID, role.ID)
			if err != nil {
				return err
			}
			if hasRole {
				count, err := s.activeAdminCount(ctx, tx, wikiID)
				if err != nil {
					return err
				}
				if count <= 1 {
					return ErrLastAdminRole
				}
			}
		}
		tag, err := tx.Exec(ctx, `DELETE FROM actor_role
			WHERE actor_id=$1 AND role_id=$2 AND wiki_id=$3`,
			targetActorID, role.ID, wikiID)
		if err != nil {
			return err
		}
		changed := tag.RowsAffected() == 1
		if changed {
			if err := s.insertRoleAudit(
				ctx, tx, actorID, targetActorID, "actor.role_revoked", role,
			); err != nil {
				return err
			}
		}
		user, err := s.getUser(ctx, tx, wikiID, targetActorID)
		if err != nil {
			return err
		}
		result = &AdminRoleMutationResult{User: *user, Changed: changed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) ensureLocalHumanUser(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT a.status
		FROM actor a JOIN local_account la ON la.actor_id=a.id
		WHERE a.id=$1 AND a.actor_type='human'`, actorID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAdminUserNotFound
	}
	return status, err
}

func (s *UserAdminService) roleByKey(
	ctx context.Context,
	tx pgx.Tx,
	roleKey string,
) (RoleSummary, error) {
	var role RoleSummary
	err := tx.QueryRow(ctx, `SELECT id,role_key,name,description
		FROM role WHERE role_key=$1`, roleKey).
		Scan(&role.ID, &role.Key, &role.Name, &role.Description)
	if errors.Is(err, pgx.ErrNoRows) {
		return RoleSummary{}, ErrAdminRoleNotFound
	}
	return role, err
}

func (s *UserAdminService) getUser(
	ctx context.Context,
	tx pgx.Tx,
	wikiID, actorID uuid.UUID,
) (*AdminUser, error) {
	row := tx.QueryRow(ctx, `
		SELECT a.id,la.username,la.email,a.display_name,a.status,a.created_at,
		       COALESCE(json_agg(json_build_object(
		           'id', r.id,
		           'key', r.role_key,
		           'name', r.name,
		           'description', r.description
		       ) ORDER BY r.role_key) FILTER (WHERE r.id IS NOT NULL), '[]'::json)
		FROM actor a
		JOIN local_account la ON la.actor_id=a.id
		LEFT JOIN actor_role ar ON ar.actor_id=a.id AND ar.wiki_id=$1
		LEFT JOIN role r ON r.id=ar.role_id
		WHERE a.id=$2 AND a.actor_type='human'
		GROUP BY a.id,la.username,la.email,a.display_name,a.status,a.created_at`,
		wikiID, actorID)
	user, err := scanAdminUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrAdminUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *UserAdminService) lockAdminRoleMutation(
	ctx context.Context,
	tx pgx.Tx,
	roleKey string,
) error {
	if roleKey != "admin" {
		return nil
	}
	_, err := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended('anby-wiki:admin-role', 0))`,
	)
	return err
}

func (s *UserAdminService) hasRole(
	ctx context.Context,
	tx pgx.Tx,
	wikiID, actorID, roleID uuid.UUID,
) (bool, error) {
	var exists bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM actor_role
		WHERE actor_id=$1 AND role_id=$2 AND wiki_id=$3
	)`, actorID, roleID, wikiID).Scan(&exists)
	return exists, err
}

func (s *UserAdminService) activeAdminCount(
	ctx context.Context,
	tx pgx.Tx,
	wikiID uuid.UUID,
) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT count(*)
		FROM actor_role ar
		JOIN role r ON r.id=ar.role_id
		JOIN actor a ON a.id=ar.actor_id
		WHERE ar.wiki_id=$1 AND r.role_key='admin'
		  AND a.actor_type='human' AND a.status='active'`, wikiID).Scan(&count)
	return count, err
}

func (s *UserAdminService) insertRoleAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID, targetActorID uuid.UUID,
	eventType string,
	role RoleSummary,
) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	payload, err := json.Marshal(map[string]any{
		"actor_id":     targetActorID,
		"role_id":      role.ID,
		"role_key":     role.Key,
		"role_name":    role.Name,
		"changed_role": role.Key,
	})
	if err != nil {
		return err
	}
	return s.repo.InsertAuditWithoutBatch(
		ctx, tx, auditID, actorID, eventType, "actor", targetActorID, payload,
	)
}

type adminUserScanner interface {
	Scan(dest ...any) error
}

func scanAdminUser(row adminUserScanner) (AdminUser, error) {
	var user AdminUser
	var rolesJSON []byte
	if err := row.Scan(
		&user.ActorID,
		&user.Username,
		&user.Email,
		&user.DisplayName,
		&user.Status,
		&user.CreatedAt,
		&rolesJSON,
	); err != nil {
		return AdminUser{}, err
	}
	if err := json.Unmarshal(rolesJSON, &user.Roles); err != nil {
		return AdminUser{}, fmt.Errorf("governance: 解析用户角色失败: %w", err)
	}
	if user.Roles == nil {
		user.Roles = []RoleSummary{}
	}
	return user, nil
}
