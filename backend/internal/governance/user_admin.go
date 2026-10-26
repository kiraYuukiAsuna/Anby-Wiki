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
	ErrLastAdminUser          = errors.New("governance: 不能删除最后一个管理员用户")
	ErrSelfAdminUserDeletion  = errors.New("governance: 不能删除当前登录用户")
	ErrWikiMemberNotFound     = errors.New("governance: Wiki 成员不存在")
	ErrInvalidWikiMember      = errors.New("governance: Wiki 成员参数非法")
	ErrLastWikiMemberRole     = errors.New("governance: 成员至少需要保留一个角色")
	ErrSelfWikiMemberRemoval  = errors.New("governance: 不能移除当前登录成员")
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

type AdminUserDeletionResult struct {
	ActorID uuid.UUID `json:"actor_id"`
	Deleted bool      `json:"deleted"`
}

type AdminUserListFilter struct {
	Search          string
	IncludeDisabled bool
}

type WikiMember = AdminUser

type WikiMemberList struct {
	Items []WikiMember `json:"items"`
}

type WikiMemberMutationResult struct {
	Member  WikiMember `json:"member"`
	Changed bool       `json:"changed"`
}

type WikiMemberRemovalResult struct {
	ActorID uuid.UUID `json:"actor_id"`
	Removed bool      `json:"removed"`
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
		WHERE a.actor_type='human' AND a.wiki_id=$1
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

func (s *UserAdminService) ListWikiMembers(
	ctx context.Context,
	wikiID, actorID uuid.UUID,
	filter AdminUserListFilter,
) (*WikiMemberList, error) {
	if err := s.auth.Check(ctx, actorID, wikiID, ActionManage, nil); err != nil {
		return nil, err
	}
	search := strings.ToLower(strings.TrimSpace(filter.Search))
	if len([]rune(search)) > 128 {
		return nil, ErrInvalidWikiMember
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
		  AND EXISTS (
		    SELECT 1 FROM actor_role member_role
		    WHERE member_role.actor_id=a.id AND member_role.wiki_id=$1
		  )
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
	items := []WikiMember{}
	for rows.Next() {
		member, err := scanAdminUser(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, member)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &WikiMemberList{Items: items}, nil
}

func (s *UserAdminService) AddWikiMember(
	ctx context.Context,
	wikiID, actorID uuid.UUID,
	identifier string,
	roleKeys []string,
) (*WikiMemberMutationResult, error) {
	roleKeys, err := cleanWikiMemberRoleKeys(roleKeys)
	if err != nil || wikiID == uuid.Nil || actorID == uuid.Nil {
		return nil, ErrInvalidWikiMember
	}
	var result *WikiMemberMutationResult
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		targetActorID, err := s.findActiveLocalHumanActor(ctx, tx, identifier)
		if err != nil {
			return err
		}
		changed := false
		for _, roleKey := range roleKeys {
			if err := s.lockAdminRoleMutation(ctx, tx, roleKey); err != nil {
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
			if tag.RowsAffected() == 1 {
				changed = true
				if err := s.insertRoleAudit(
					ctx, tx, actorID, targetActorID, "wiki.member_role_granted", role,
				); err != nil {
					return err
				}
			}
		}
		member, err := s.getWikiMember(ctx, tx, wikiID, targetActorID)
		if err != nil {
			return err
		}
		result = &WikiMemberMutationResult{Member: *member, Changed: changed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) GrantWikiMemberRole(
	ctx context.Context,
	wikiID, actorID, targetActorID uuid.UUID,
	roleKey string,
) (*WikiMemberMutationResult, error) {
	roleKey = strings.TrimSpace(roleKey)
	if wikiID == uuid.Nil || actorID == uuid.Nil ||
		targetActorID == uuid.Nil || roleKey == "" {
		return nil, ErrInvalidWikiMember
	}
	var result *WikiMemberMutationResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		if _, err := s.getWikiMember(ctx, tx, wikiID, targetActorID); err != nil {
			return err
		}
		if err := s.lockAdminRoleMutation(ctx, tx, roleKey); err != nil {
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
				ctx, tx, actorID, targetActorID, "wiki.member_role_granted", role,
			); err != nil {
				return err
			}
		}
		member, err := s.getWikiMember(ctx, tx, wikiID, targetActorID)
		if err != nil {
			return err
		}
		result = &WikiMemberMutationResult{Member: *member, Changed: changed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) RevokeWikiMemberRole(
	ctx context.Context,
	wikiID, actorID, targetActorID uuid.UUID,
	roleKey string,
) (*WikiMemberMutationResult, error) {
	roleKey = strings.TrimSpace(roleKey)
	if wikiID == uuid.Nil || actorID == uuid.Nil ||
		targetActorID == uuid.Nil || roleKey == "" {
		return nil, ErrInvalidWikiMember
	}
	var result *WikiMemberMutationResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		member, err := s.getWikiMember(ctx, tx, wikiID, targetActorID)
		if err != nil {
			return err
		}
		if err := s.lockAdminRoleMutation(ctx, tx, roleKey); err != nil {
			return err
		}
		role, err := s.roleByKey(ctx, tx, roleKey)
		if err != nil {
			return err
		}
		hasRole, err := s.hasRole(ctx, tx, wikiID, targetActorID, role.ID)
		if err != nil {
			return err
		}
		if hasRole {
			count, err := s.wikiMemberRoleCount(ctx, tx, wikiID, targetActorID)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastWikiMemberRole
			}
			if role.Key == "admin" && member.Status == "active" {
				adminCount, err := s.activeAdminCount(ctx, tx, wikiID)
				if err != nil {
					return err
				}
				if adminCount <= 1 {
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
				ctx, tx, actorID, targetActorID, "wiki.member_role_revoked", role,
			); err != nil {
				return err
			}
		}
		member, err = s.getWikiMember(ctx, tx, wikiID, targetActorID)
		if err != nil {
			return err
		}
		result = &WikiMemberMutationResult{Member: *member, Changed: changed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UserAdminService) RemoveWikiMember(
	ctx context.Context,
	wikiID, actorID, targetActorID uuid.UUID,
) (*WikiMemberRemovalResult, error) {
	if wikiID == uuid.Nil || actorID == uuid.Nil || targetActorID == uuid.Nil {
		return nil, ErrInvalidWikiMember
	}
	var result *WikiMemberRemovalResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		if actorID == targetActorID {
			return ErrSelfWikiMemberRemoval
		}
		if err := s.lockAdminRoleMutation(ctx, tx, "admin"); err != nil {
			return err
		}
		member, err := s.getWikiMember(ctx, tx, wikiID, targetActorID)
		if err != nil {
			return err
		}
		adminRole, err := s.roleByKey(ctx, tx, "admin")
		if err != nil {
			return err
		}
		hasAdmin, err := s.hasRole(ctx, tx, wikiID, targetActorID, adminRole.ID)
		if err != nil {
			return err
		}
		if member.Status == "active" && hasAdmin {
			count, err := s.activeAdminCount(ctx, tx, wikiID)
			if err != nil {
				return err
			}
			if count <= 1 {
				return ErrLastAdminUser
			}
		}
		tag, err := tx.Exec(ctx, `DELETE FROM actor_role
			WHERE actor_id=$1 AND wiki_id=$2`, targetActorID, wikiID)
		if err != nil {
			return err
		}
		removed := tag.RowsAffected() > 0
		if removed {
			if err := s.insertMemberRemovalAudit(ctx, tx, actorID, *member); err != nil {
				return err
			}
		}
		result = &WikiMemberRemovalResult{ActorID: targetActorID, Removed: removed}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
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
		if _, err := s.ensureLocalHumanUser(
			ctx, tx, wikiID, targetActorID,
		); err != nil {
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
		targetStatus, err := s.ensureLocalHumanUser(
			ctx, tx, wikiID, targetActorID,
		)
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

func (s *UserAdminService) DeleteUser(
	ctx context.Context,
	wikiID, actorID, targetActorID uuid.UUID,
) (*AdminUserDeletionResult, error) {
	if wikiID == uuid.Nil || actorID == uuid.Nil || targetActorID == uuid.Nil {
		return nil, ErrInvalidAdminUserFilter
	}
	var result *AdminUserDeletionResult
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := s.auth.CheckTx(ctx, tx, actorID, wikiID, ActionManage, nil); err != nil {
			return err
		}
		if actorID == targetActorID {
			return ErrSelfAdminUserDeletion
		}
		if err := s.lockAdminRoleMutation(ctx, tx, "admin"); err != nil {
			return err
		}
		targetStatus, err := s.lockLocalHumanUser(
			ctx, tx, wikiID, targetActorID,
		)
		if err != nil {
			return err
		}
		user, err := s.getUser(ctx, tx, wikiID, targetActorID)
		if err != nil {
			return err
		}
		adminRole, err := s.roleByKey(ctx, tx, "admin")
		if err != nil {
			return err
		}
		if targetStatus == "active" {
			hasAdmin, err := s.hasRole(ctx, tx, wikiID, targetActorID, adminRole.ID)
			if err != nil {
				return err
			}
			if hasAdmin {
				count, err := s.activeAdminCount(ctx, tx, wikiID)
				if err != nil {
					return err
				}
				if count <= 1 {
					return ErrLastAdminUser
				}
			}
		}
		deleted := false
		for _, statement := range []string{
			`DELETE FROM cli_authorization_code WHERE actor_id=$1`,
			`DELETE FROM cli_access_token WHERE actor_id=$1`,
			`DELETE FROM auth_session WHERE actor_id=$1`,
			`DELETE FROM actor_role WHERE actor_id=$1`,
		} {
			tag, err := tx.Exec(ctx, statement, targetActorID)
			if err != nil {
				return err
			}
			deleted = deleted || tag.RowsAffected() > 0
		}
		tag, err := tx.Exec(ctx, `UPDATE actor
			SET status='disabled', user_id=NULL
			WHERE id=$1 AND (status<>'disabled' OR user_id IS NOT NULL)`,
			targetActorID)
		if err != nil {
			return err
		}
		deleted = deleted || tag.RowsAffected() > 0
		tag, err = tx.Exec(ctx, `DELETE FROM local_account WHERE actor_id=$1`,
			targetActorID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrAdminUserNotFound
		}
		deleted = true
		if err := s.insertUserDeletionAudit(ctx, tx, actorID, *user); err != nil {
			return err
		}
		result = &AdminUserDeletionResult{ActorID: targetActorID, Deleted: deleted}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func cleanWikiMemberRoleKeys(roleKeys []string) ([]string, error) {
	seen := map[string]bool{}
	result := []string{}
	for _, roleKey := range roleKeys {
		roleKey = strings.TrimSpace(roleKey)
		if roleKey == "" {
			return nil, ErrInvalidWikiMember
		}
		if !seen[roleKey] {
			seen[roleKey] = true
			result = append(result, roleKey)
		}
	}
	if len(result) == 0 || len(result) > 4 {
		return nil, ErrInvalidWikiMember
	}
	return result, nil
}

func (s *UserAdminService) findActiveLocalHumanActor(
	ctx context.Context,
	tx pgx.Tx,
	identifier string,
) (uuid.UUID, error) {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	if identifier == "" || len([]rune(identifier)) > 254 {
		return uuid.Nil, ErrInvalidWikiMember
	}
	var actorID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT a.id
		FROM actor a
		JOIN local_account la ON la.actor_id=a.id
		WHERE a.actor_type='human'
		  AND a.status='active'
		  AND la.status='active'
		  AND (
		    la.normalized_username=$1
		    OR la.normalized_email=$1
		    OR a.id::text=$1
		  )
		FOR UPDATE OF a, la`, identifier).Scan(&actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrWikiMemberNotFound
	}
	return actorID, err
}

func (s *UserAdminService) getWikiMember(
	ctx context.Context,
	tx pgx.Tx,
	wikiID, actorID uuid.UUID,
) (*WikiMember, error) {
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
		  AND EXISTS (
		    SELECT 1 FROM actor_role member_role
		    WHERE member_role.actor_id=a.id AND member_role.wiki_id=$1
		  )
		GROUP BY a.id,la.username,la.email,a.display_name,a.status,a.created_at`,
		wikiID, actorID)
	member, err := scanAdminUser(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrWikiMemberNotFound
	}
	if err != nil {
		return nil, err
	}
	return &member, nil
}

func (s *UserAdminService) wikiMemberRoleCount(
	ctx context.Context,
	tx pgx.Tx,
	wikiID, actorID uuid.UUID,
) (int, error) {
	var count int
	err := tx.QueryRow(ctx, `SELECT count(*)
		FROM actor_role
		WHERE actor_id=$1 AND wiki_id=$2`, actorID, wikiID).Scan(&count)
	return count, err
}

func (s *UserAdminService) ensureLocalHumanUser(
	ctx context.Context,
	tx pgx.Tx,
	wikiID, actorID uuid.UUID,
) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT a.status
		FROM actor a JOIN local_account la ON la.actor_id=a.id
		WHERE a.id=$1 AND a.wiki_id=$2 AND a.actor_type='human'`,
		actorID, wikiID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrAdminUserNotFound
	}
	return status, err
}

func (s *UserAdminService) lockLocalHumanUser(
	ctx context.Context,
	tx pgx.Tx,
	wikiID, actorID uuid.UUID,
) (string, error) {
	var status string
	err := tx.QueryRow(ctx, `SELECT a.status
		FROM actor a JOIN local_account la ON la.actor_id=a.id
		WHERE a.id=$1 AND a.wiki_id=$2 AND a.actor_type='human'
		FOR UPDATE OF a, la`, actorID, wikiID).Scan(&status)
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
		WHERE a.id=$2 AND a.wiki_id=$1 AND a.actor_type='human'
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

func (s *UserAdminService) insertUserDeletionAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	user AdminUser,
) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	roles := make([]string, 0, len(user.Roles))
	for _, role := range user.Roles {
		roles = append(roles, role.Key)
	}
	payload, err := json.Marshal(map[string]any{
		"actor_id":     user.ActorID,
		"username":     user.Username,
		"display_name": user.DisplayName,
		"status":       user.Status,
		"roles":        roles,
	})
	if err != nil {
		return err
	}
	return s.repo.InsertAuditWithoutBatch(
		ctx, tx, auditID, actorID, "actor.deleted", "actor", user.ActorID, payload,
	)
}

func (s *UserAdminService) insertMemberRemovalAudit(
	ctx context.Context,
	tx pgx.Tx,
	actorID uuid.UUID,
	member WikiMember,
) error {
	auditID, err := s.ids.New()
	if err != nil {
		return err
	}
	roles := make([]string, 0, len(member.Roles))
	for _, role := range member.Roles {
		roles = append(roles, role.Key)
	}
	payload, err := json.Marshal(map[string]any{
		"actor_id":     member.ActorID,
		"username":     member.Username,
		"display_name": member.DisplayName,
		"status":       member.Status,
		"roles":        roles,
	})
	if err != nil {
		return err
	}
	return s.repo.InsertAuditWithoutBatch(
		ctx, tx, auditID, actorID, "wiki.member_removed", "actor", member.ActorID, payload,
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
