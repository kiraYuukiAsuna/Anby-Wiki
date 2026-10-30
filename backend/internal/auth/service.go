package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/argon2"

	"github.com/anby/wiki/backend/internal/platform/db"
)

var (
	ErrUnauthenticated    = errors.New("auth: unauthenticated")
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrAccountExists      = errors.New("auth: account already exists")
	ErrAccountNotFound    = errors.New("auth: account not found")
)

const (
	passwordMinRunes = 12
	passwordMaxRunes = 128
	argonMemory      = 64 * 1024
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLength  = 16
	argonKeyLength   = 32
)

const (
	wikiMembershipRolePageViewer = "page_viewer"
	wikiMembershipRolePageEditor = "page_editor"
	pageShareRoleEditor          = "editor"
)

var (
	usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{2,31}$`)
	dummyHashOnce   sync.Once
	dummyHash       string
)

// ValidationError represents a safe, user-facing account input error.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return "auth: invalid " + e.Field + ": " + e.Message
}

type idGenerator interface {
	New() (uuid.UUID, error)
}

// Identity is a verified external identity. It is retained for future identity
// provider adapters; local password accounts use Register and Login.
type Identity struct {
	Issuer      string
	Subject     string
	DisplayName string
}

// RegisterInput contains the public local-account registration fields.
type RegisterInput struct {
	Username    string
	Email       string
	Password    string
	DisplayName string
}

// LoginInput accepts either a username or an email address.
type LoginInput struct {
	Identifier string
	Password   string
}

// Profile is the editable account profile for the current local account.
type Profile struct {
	ActorID     uuid.UUID
	WikiID      uuid.UUID
	Username    string
	Email       string
	DisplayName string
	WikiName    string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// UpdateProfileInput contains current-account profile fields.
type UpdateProfileInput struct {
	ActorID     uuid.UUID
	WikiID      uuid.UUID
	Email       string
	DisplayName string
	WikiName    string
}

// Session contains the opaque cookie value. Only its SHA-256 hash is stored.
type Session struct {
	Token           string
	ActorID         uuid.UUID
	WikiID          uuid.UUID
	WikiName        string
	MainNamespaceID uuid.UUID
	DisplayName     string
	ExpiresAt       time.Time
}

// WikiMembership is one Wiki context the actor may select for requests.
type WikiMembership struct {
	WikiID          uuid.UUID
	WikiName        string
	MainNamespaceID uuid.UUID
	Roles           []string
}

// Service is the sole write path for accounts, external identities and
// authentication sessions.
type Service struct {
	pool       db.Querier
	txm        *db.TxManager
	ids        idGenerator
	sessionTTL time.Duration
	now        func() time.Time
}

func NewService(pool db.Querier, txm *db.TxManager, ids idGenerator, sessionTTL time.Duration) *Service {
	return &Service{
		pool:       pool,
		txm:        txm,
		ids:        ids,
		sessionTTL: sessionTTL,
		now:        time.Now,
	}
}

// Register creates a local account, its private Wiki, owner Actor, admin role
// and first session in one transaction.
func (s *Service) Register(ctx context.Context, input RegisterInput) (Session, error) {
	username, email, displayName, err := validateRegistration(input)
	if err != nil {
		return Session{}, err
	}
	passwordHash, err := hashPassword(input.Password)
	if err != nil {
		return Session{}, err
	}
	accountID, err := s.ids.New()
	if err != nil {
		return Session{}, fmt.Errorf("auth: generate account id: %w", err)
	}
	actorID, err := s.ids.New()
	if err != nil {
		return Session{}, fmt.Errorf("auth: generate actor id: %w", err)
	}
	wiki, err := s.newPersonalWiki(actorID, displayName)
	if err != nil {
		return Session{}, err
	}
	sessionID, session, hash, err := s.newSession(
		actorID, wiki.ID, wiki.Name, wiki.MainNamespaceID, displayName,
	)
	if err != nil {
		return Session{}, err
	}

	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if err := insertPersonalWiki(ctx, tx, wiki); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO actor
			(id,actor_type,wiki_id,display_name,status)
			VALUES ($1,'human',$2,$3,'active')`,
			actorID, wiki.ID, displayName); err != nil {
			return err
		}
		if err := insertPersonalWikiDefaults(ctx, tx, wiki); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO local_account
			(id,actor_id,username,normalized_username,email,normalized_email,password_hash)
			VALUES ($1,$2,$3,$3,$4,$5,$6)`,
			accountID, actorID, username, email, strings.ToLower(email), passwordHash); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE actor SET user_id=$1 WHERE id=$2`,
			accountID, actorID); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO actor_role (actor_id,role_id,wiki_id)
			SELECT $1,r.id,$2 FROM role r WHERE r.role_key='admin'`,
			actorID, wiki.ID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return errors.New("auth: admin role is missing")
		}
		_, err = tx.Exec(ctx, `INSERT INTO auth_session
			(id,token_hash,actor_id,expires_at) VALUES ($1,$2,$3,$4)`,
			sessionID, hash[:], actorID, session.ExpiresAt)
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Session{}, ErrAccountExists
		}
		return Session{}, fmt.Errorf("auth: register account: %w", err)
	}
	return session, nil
}

// Login verifies a local account password and creates a fresh opaque session.
// All lookup, password and account-state failures deliberately share one error.
func (s *Service) Login(ctx context.Context, input LoginInput) (Session, error) {
	identifier := strings.ToLower(strings.TrimSpace(input.Identifier))
	if identifier == "" || len(identifier) > 254 || len(input.Password) > 512 {
		verifyDummyPassword(input.Password)
		return Session{}, ErrInvalidCredentials
	}

	var accountID, actorID, wikiID, mainNamespaceID uuid.UUID
	var passwordHash, displayName, wikiName, accountStatus, actorStatus string
	err := s.pool.QueryRow(ctx, `SELECT
		la.id,la.actor_id,a.wiki_id,n.id,la.password_hash,a.display_name,w.name,
		la.status,a.status
		FROM local_account la
		JOIN actor a ON a.id=la.actor_id
		JOIN wiki_site w ON w.id=a.wiki_id
		JOIN namespace n ON n.wiki_id=a.wiki_id AND n.namespace_key='main'
		WHERE la.normalized_username=$1 OR la.normalized_email=$1`,
		identifier).Scan(
		&accountID, &actorID, &wikiID, &mainNamespaceID,
		&passwordHash, &displayName, &wikiName,
		&accountStatus, &actorStatus,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		verifyDummyPassword(input.Password)
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, fmt.Errorf("auth: lookup account: %w", err)
	}
	valid, err := verifyPassword(input.Password, passwordHash)
	if err != nil {
		return Session{}, fmt.Errorf("auth: verify password hash: %w", err)
	}
	if !valid || accountStatus != "active" || actorStatus != "active" {
		return Session{}, ErrInvalidCredentials
	}

	sessionID, session, hash, err := s.newSession(
		actorID, wikiID, wikiName, mainNamespaceID, displayName,
	)
	if err != nil {
		return Session{}, err
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		var currentDisplayName string
		err := tx.QueryRow(ctx, `UPDATE local_account la
			SET last_login_at=now(),updated_at=now()
			FROM actor a
			WHERE la.id=$1 AND a.id=la.actor_id
				AND la.status='active' AND a.status='active'
			RETURNING a.display_name`, accountID).Scan(&currentDisplayName)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidCredentials
		}
		if err != nil {
			return err
		}
		session.DisplayName = currentDisplayName
		_, err = tx.Exec(ctx, `INSERT INTO auth_session
			(id,token_hash,actor_id,expires_at) VALUES ($1,$2,$3,$4)`,
			sessionID, hash[:], actorID, session.ExpiresAt)
		return err
	})
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			return Session{}, ErrInvalidCredentials
		}
		return Session{}, fmt.Errorf("auth: create login session: %w", err)
	}
	return session, nil
}

// EstablishSession maps a verified external identity to one Actor. This is not
// exposed as a shared-secret login route and exists for future IdP adapters.
func (s *Service) EstablishSession(ctx context.Context, identity Identity) (Session, error) {
	identity.Issuer = strings.TrimSpace(identity.Issuer)
	identity.Subject = strings.TrimSpace(identity.Subject)
	identity.DisplayName = strings.TrimSpace(identity.DisplayName)
	if identity.Issuer == "" || identity.Subject == "" {
		return Session{}, fmt.Errorf("auth: verified identity lacks issuer or subject")
	}
	if identity.DisplayName == "" {
		identity.DisplayName = "Unnamed user"
	}
	sessionID, session, hash, err := s.newSession(
		uuid.Nil, uuid.Nil, "", uuid.Nil, identity.DisplayName,
	)
	if err != nil {
		return Session{}, err
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			identity.Issuer+"\x1f"+identity.Subject); err != nil {
			return err
		}
		var actorStatus string
		err := tx.QueryRow(ctx, `SELECT
			ei.actor_id,a.wiki_id,w.name,n.id,a.status,a.display_name
			FROM external_identity ei
			JOIN actor a ON a.id=ei.actor_id
			JOIN wiki_site w ON w.id=a.wiki_id
			JOIN namespace n ON n.wiki_id=a.wiki_id AND n.namespace_key='main'
			WHERE ei.issuer=$1 AND ei.subject=$2`,
			identity.Issuer, identity.Subject).Scan(
			&session.ActorID, &session.WikiID, &session.WikiName,
			&session.MainNamespaceID,
			&actorStatus, &session.DisplayName,
		)
		switch {
		case err == nil:
			if actorStatus != "active" {
				return ErrUnauthenticated
			}
			if _, err := tx.Exec(ctx, `UPDATE external_identity SET last_seen_at=now()
				WHERE issuer=$1 AND subject=$2`, identity.Issuer, identity.Subject); err != nil {
				return err
			}
		case errors.Is(err, pgx.ErrNoRows):
			session.ActorID, err = s.ids.New()
			if err != nil {
				return err
			}
			wiki, err := s.newPersonalWiki(session.ActorID, identity.DisplayName)
			if err != nil {
				return err
			}
			if err := insertPersonalWiki(ctx, tx, wiki); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO actor
				(id,actor_type,wiki_id,display_name,status)
				VALUES ($1,'human',$2,$3,'active')`,
				session.ActorID, wiki.ID, identity.DisplayName); err != nil {
				return err
			}
			if err := insertPersonalWikiDefaults(ctx, tx, wiki); err != nil {
				return err
			}
			session.DisplayName = identity.DisplayName
			session.WikiID = wiki.ID
			session.WikiName = wiki.Name
			session.MainNamespaceID = wiki.MainNamespaceID
			if _, err := tx.Exec(ctx, `INSERT INTO external_identity
				(issuer,subject,actor_id) VALUES ($1,$2,$3)`,
				identity.Issuer, identity.Subject, session.ActorID); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `INSERT INTO actor_role (actor_id,role_id,wiki_id)
				SELECT $1,r.id,$2 FROM role r WHERE r.role_key='admin'`,
				session.ActorID, wiki.ID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() != 1 {
				return errors.New("auth: admin role is missing")
			}
		default:
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO auth_session
			(id,token_hash,actor_id,expires_at) VALUES ($1,$2,$3,$4)`,
			sessionID, hash[:], session.ActorID, session.ExpiresAt)
		return err
	})
	if err != nil {
		return Session{}, fmt.Errorf("auth: establish session: %w", err)
	}
	return session, nil
}

// ResolveSession authenticates an opaque token and re-reads Actor status on
// every request. No role or permission is cached in the session.
func (s *Service) ResolveSession(ctx context.Context, token string, requestedWikiID uuid.UUID) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	hash := tokenHash(token)
	var actorID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT actor_id
		FROM auth_session
		WHERE token_hash=$1 AND revoked_at IS NULL AND expires_at>now()`,
		hash[:],
	).Scan(&actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: resolve session: %w", err)
	}
	return s.resolveActorPrincipal(
		ctx, actorID, requestedWikiID, uuid.Nil, "session",
	)
}

// ResolveDevActor validates an explicitly supplied development/test Actor.
func (s *Service) ResolveDevActor(ctx context.Context, actorID, requestedWikiID uuid.UUID) (Principal, error) {
	return s.resolveActorPrincipal(
		ctx, actorID, requestedWikiID, uuid.Nil, "development_header",
	)
}

func (s *Service) resolveActorPrincipal(
	ctx context.Context,
	actorID, requestedWikiID, credentialID uuid.UUID,
	method string,
) (Principal, error) {
	targetWikiID := requestedWikiID
	if targetWikiID == uuid.Nil {
		var err error
		targetWikiID, err = s.actorHomeWikiID(ctx, actorID)
		if err != nil {
			return Principal{}, err
		}
	}
	var principal Principal
	err := s.pool.QueryRow(ctx, `SELECT
		a.id,w.id,COALESCE(a.wiki_id,$3),w.name,n.id,a.actor_type,a.display_name,
		COALESCE(la.username,'')
		FROM actor a
		JOIN wiki_site w ON w.id=$2
		JOIN namespace n ON n.wiki_id=w.id AND n.namespace_key='main'
		LEFT JOIN local_account la ON la.actor_id=a.id
		WHERE a.id=$1 AND a.status='active'
			AND (la.id IS NULL OR la.status='active')
			AND (
				a.actor_type='system'
				OR (a.actor_type IN ('ai','import') AND a.wiki_id=w.id
				  AND starts_with(a.external_key,$4)
				  AND EXISTS (SELECT 1 FROM local_account owner_account
				    JOIN actor owner ON owner.id=owner_account.actor_id
				    JOIN actor_role ar ON ar.actor_id=owner.id AND ar.wiki_id=w.id
				    JOIN role role ON role.id=ar.role_id AND role.role_key='admin'
				    WHERE owner_account.id=a.user_id AND owner_account.status='active'
				      AND owner.status='active' AND owner.actor_type='human'))
				OR (a.actor_type NOT IN ('ai','import') AND (EXISTS (
					SELECT 1 FROM actor_role ar
					WHERE ar.actor_id=a.id AND ar.wiki_id=w.id
				)
				OR EXISTS (
					SELECT 1
					FROM page_share ps
					JOIN page p ON p.id=ps.page_id
					WHERE ps.actor_id=a.id AND p.wiki_id=w.id
					  AND p.deleted_at IS NULL
				)))
			)`, actorID, targetWikiID, uuid.Nil, MachineActorKeyPrefix).Scan(
		&principal.ActorID, &principal.WikiID, &principal.HomeWikiID, &principal.WikiName,
		&principal.MainNamespaceID,
		&principal.ActorType, &principal.DisplayName, &principal.Username,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: resolve actor principal: %w", err)
	}
	principal.CredentialID = credentialID
	principal.Method = method
	return principal, nil
}

func (s *Service) actorHomeWikiID(ctx context.Context, actorID uuid.UUID) (uuid.UUID, error) {
	var wikiID uuid.UUID
	err := s.pool.QueryRow(ctx, `SELECT wiki_id
		FROM actor
		WHERE id=$1 AND status='active' AND wiki_id IS NOT NULL`, actorID).Scan(&wikiID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrUnauthenticated
	}
	if err != nil {
		return uuid.Nil, fmt.Errorf("auth: resolve actor home wiki: %w", err)
	}
	return wikiID, nil
}

// ListWikiMemberships returns all Wiki contexts this Actor may select.
func (s *Service) ListWikiMemberships(ctx context.Context, actorID uuid.UUID) ([]WikiMembership, error) {
	if actorID == uuid.Nil {
		return nil, ErrUnauthenticated
	}
	var actorType string
	if err := s.pool.QueryRow(ctx, `SELECT actor_type FROM actor WHERE id=$1`, actorID).Scan(&actorType); err != nil {
		return nil, ErrUnauthenticated
	}
	if actorType == ActorTypeAI || actorType == ActorTypeImport {
		principal, err := s.resolveActorPrincipal(ctx, actorID, uuid.Nil, uuid.Nil, "")
		if err != nil {
			return nil, err
		}
		return []WikiMembership{{
			WikiID: principal.WikiID, WikiName: principal.WikiName,
			MainNamespaceID: principal.MainNamespaceID, Roles: []string{},
		}}, nil
	}
	rows, err := s.pool.Query(ctx, `SELECT
		wiki_id,wiki_name,namespace_id,role_key
		FROM (
			SELECT w.id AS wiki_id,w.name AS wiki_name,n.id AS namespace_id,
			       r.role_key
			FROM actor_role ar
			JOIN actor a ON a.id=ar.actor_id
			JOIN wiki_site w ON w.id=ar.wiki_id
			JOIN namespace n ON n.wiki_id=w.id AND n.namespace_key='main'
			JOIN role r ON r.id=ar.role_id
			LEFT JOIN local_account la ON la.actor_id=a.id
			WHERE ar.actor_id=$1 AND a.status='active'
				AND (la.id IS NULL OR la.status='active')
			UNION
			SELECT w.id AS wiki_id,w.name AS wiki_name,n.id AS namespace_id,
			       CASE
			         WHEN bool_or(ps.role_key=$4) THEN $2
			         ELSE $3
			       END AS role_key
			FROM page_share ps
			JOIN page p ON p.id=ps.page_id
			JOIN wiki_site w ON w.id=p.wiki_id
			JOIN namespace n ON n.wiki_id=w.id AND n.namespace_key='main'
			JOIN actor a ON a.id=ps.actor_id
			LEFT JOIN local_account la ON la.actor_id=a.id
			WHERE ps.actor_id=$1 AND p.deleted_at IS NULL
				AND a.status='active'
				AND (la.id IS NULL OR la.status='active')
			GROUP BY w.id,w.name,n.id
		) memberships
		ORDER BY lower(wiki_name),wiki_id,role_key`,
		actorID, wikiMembershipRolePageEditor, wikiMembershipRolePageViewer,
		pageShareRoleEditor)
	if err != nil {
		return nil, fmt.Errorf("auth: list wiki memberships: %w", err)
	}
	defer rows.Close()
	items := []WikiMembership{}
	var current *WikiMembership
	for rows.Next() {
		var wikiID, namespaceID uuid.UUID
		var wikiName, role string
		if err := rows.Scan(&wikiID, &wikiName, &namespaceID, &role); err != nil {
			return nil, fmt.Errorf("auth: scan wiki membership: %w", err)
		}
		if current == nil || current.WikiID != wikiID {
			items = append(items, WikiMembership{
				WikiID: wikiID, WikiName: wikiName,
				MainNamespaceID: namespaceID,
				Roles:           []string{},
			})
			current = &items[len(items)-1]
		}
		current.Roles = append(current.Roles, role)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("auth: iterate wiki memberships: %w", err)
	}
	return items, nil
}

// RevokeSession immediately invalidates the supplied opaque token.
func (s *Service) RevokeSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	hash := tokenHash(token)
	if _, err := s.pool.Exec(ctx, `UPDATE auth_session SET revoked_at=now()
		WHERE token_hash=$1 AND revoked_at IS NULL`, hash[:]); err != nil {
		return fmt.Errorf("auth: revoke session: %w", err)
	}
	return nil
}

// GetProfile returns the local account profile for the authenticated Actor.
func (s *Service) GetProfile(ctx context.Context, actorID, _ uuid.UUID) (Profile, error) {
	profile, err := s.scanProfile(ctx, s.pool, actorID)
	if err != nil {
		return Profile{}, err
	}
	return profile, nil
}

// UpdateProfile updates only fields owned by the current local account.
func (s *Service) UpdateProfile(ctx context.Context, input UpdateProfileInput) (Profile, error) {
	email, normalizedEmail, displayName, wikiName, err := validateProfileUpdate(input)
	if err != nil {
		return Profile{}, err
	}
	var profile Profile
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE actor
			SET display_name=$2
			WHERE id=$1 AND status='active'`,
			input.ActorID, displayName)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrAccountNotFound
		}
		tag, err = tx.Exec(ctx, `UPDATE local_account
			SET email=$2, normalized_email=$3, updated_at=now()
			WHERE actor_id=$1 AND status='active'
				AND EXISTS (
					SELECT 1 FROM actor a
					WHERE a.id=local_account.actor_id
						AND a.status='active'
				)`,
			input.ActorID, email, normalizedEmail)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrAccountNotFound
		}
		tag, err = tx.Exec(ctx, `UPDATE wiki_site
			SET name=$2
			WHERE owner_actor_id=$1`,
			input.ActorID, wikiName)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return ErrAccountNotFound
		}
		profile, err = s.scanProfile(ctx, tx, input.ActorID)
		return err
	})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return Profile{}, ErrAccountExists
		}
		return Profile{}, fmt.Errorf("auth: update profile: %w", err)
	}
	return profile, nil
}

func (s *Service) scanProfile(
	ctx context.Context,
	querier db.Querier,
	actorID uuid.UUID,
) (Profile, error) {
	var profile Profile
	err := querier.QueryRow(ctx, `SELECT
		a.id,a.wiki_id,la.username,la.email,a.display_name,w.name,
		la.created_at,la.updated_at
		FROM actor a
		JOIN local_account la ON la.actor_id=a.id
		JOIN wiki_site w ON w.id=a.wiki_id
		WHERE a.id=$1
			AND a.status='active' AND la.status='active'`,
		actorID).Scan(
		&profile.ActorID, &profile.WikiID, &profile.Username, &profile.Email,
		&profile.DisplayName, &profile.WikiName, &profile.CreatedAt, &profile.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrAccountNotFound
	}
	if err != nil {
		return Profile{}, fmt.Errorf("auth: read profile: %w", err)
	}
	return profile, nil
}

func (s *Service) newSession(
	actorID, wikiID uuid.UUID,
	wikiName string,
	mainNamespaceID uuid.UUID,
	displayName string,
) (uuid.UUID, Session, [sha256.Size]byte, error) {
	token, err := randomToken(32)
	if err != nil {
		return uuid.Nil, Session{}, [sha256.Size]byte{}, err
	}
	sessionID, err := s.ids.New()
	if err != nil {
		return uuid.Nil, Session{}, [sha256.Size]byte{}, fmt.Errorf("auth: generate session id: %w", err)
	}
	session := Session{
		Token:           token,
		ActorID:         actorID,
		WikiID:          wikiID,
		WikiName:        wikiName,
		MainNamespaceID: mainNamespaceID,
		DisplayName:     displayName,
		ExpiresAt:       s.now().Add(s.sessionTTL),
	}
	return sessionID, session, tokenHash(token), nil
}

type personalWiki struct {
	ID              uuid.UUID
	OwnerID         uuid.UUID
	SiteKey         string
	Name            string
	MainNamespaceID uuid.UUID
	Namespaces      []personalNamespace
	Tags            []personalChangeTag
	ComponentID     uuid.UUID
}

type personalNamespace struct {
	ID            uuid.UUID
	Key           string
	CanonicalName string
	DisplayName   string
	IsContent     bool
}

type personalChangeTag struct {
	ID          uuid.UUID
	Key         string
	Name        string
	Description string
}

var personalNamespaceDefinitions = []personalNamespace{
	{Key: "main", CanonicalName: "Main", DisplayName: "条目", IsContent: true},
	{Key: "talk", CanonicalName: "Talk", DisplayName: "讨论"},
	{Key: "user", CanonicalName: "User", DisplayName: "用户"},
	{Key: "project", CanonicalName: "Project", DisplayName: "项目"},
	{Key: "component", CanonicalName: "Component", DisplayName: "组件", IsContent: true},
	{Key: "collection", CanonicalName: "Collection", DisplayName: "合集", IsContent: true},
	{Key: "file", CanonicalName: "File", DisplayName: "文件", IsContent: true},
}

var personalChangeTagDefinitions = []personalChangeTag{
	{Key: "minor", Name: "小修改", Description: "不改变核心语义的小型整理"},
	{Key: "ai-assisted", Name: "AI 协助", Description: "由 AI 生成或协助完成的变更"},
	{Key: "imported", Name: "批量导入", Description: "由来源导入管道生成的变更"},
	{Key: "reverted", Name: "已回滚", Description: "已通过补偿版本撤销的变更"},
	{Key: "high-risk", Name: "高风险", Description: "需完整人工审核的高风险变更"},
	{Key: "human-verified", Name: "人工核验", Description: "已由人工复核证据或事实"},
}

func (s *Service) newPersonalWiki(
	ownerID uuid.UUID,
	displayName string,
) (personalWiki, error) {
	wikiID, err := s.ids.New()
	if err != nil {
		return personalWiki{}, fmt.Errorf("auth: generate wiki id: %w", err)
	}
	result := personalWiki{
		ID: wikiID, OwnerID: ownerID, SiteKey: "user-" + ownerID.String(),
		Name:       strings.TrimSpace(displayName) + " 的 Wiki",
		Namespaces: make([]personalNamespace, len(personalNamespaceDefinitions)),
		Tags:       make([]personalChangeTag, len(personalChangeTagDefinitions)),
	}
	result.ComponentID, err = s.ids.New()
	if err != nil {
		return personalWiki{}, fmt.Errorf("auth: generate component id: %w", err)
	}
	for index, definition := range personalNamespaceDefinitions {
		namespaceID, err := s.ids.New()
		if err != nil {
			return personalWiki{}, fmt.Errorf("auth: generate namespace id: %w", err)
		}
		result.Namespaces[index] = definition
		result.Namespaces[index].ID = namespaceID
		if definition.Key == "main" {
			result.MainNamespaceID = namespaceID
		}
	}
	for index, definition := range personalChangeTagDefinitions {
		tagID, err := s.ids.New()
		if err != nil {
			return personalWiki{}, fmt.Errorf("auth: generate change tag id: %w", err)
		}
		result.Tags[index] = definition
		result.Tags[index].ID = tagID
	}
	return result, nil
}

func insertPersonalWiki(
	ctx context.Context,
	tx pgx.Tx,
	wiki personalWiki,
) error {
	if _, err := tx.Exec(ctx, `INSERT INTO wiki_site
		(id,site_key,name,default_language,owner_actor_id)
		VALUES ($1,$2,$3,'zh-Hans',$4)`,
		wiki.ID, wiki.SiteKey, wiki.Name, wiki.OwnerID); err != nil {
		return err
	}
	for _, namespace := range wiki.Namespaces {
		if _, err := tx.Exec(ctx, `INSERT INTO namespace
			(id,wiki_id,namespace_key,canonical_name,display_name,is_content)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			namespace.ID, wiki.ID, namespace.Key, namespace.CanonicalName,
			namespace.DisplayName, namespace.IsContent); err != nil {
			return err
		}
	}
	for _, tag := range wiki.Tags {
		if _, err := tx.Exec(ctx, `INSERT INTO change_tag
			(id,wiki_id,tag_key,name,description) VALUES ($1,$2,$3,$4,$5)`,
			tag.ID, wiki.ID, tag.Key, tag.Name, tag.Description); err != nil {
			return err
		}
	}
	return nil
}

func insertPersonalWikiDefaults(
	ctx context.Context,
	tx pgx.Tx,
	wiki personalWiki,
) error {
	if _, err := tx.Exec(ctx, `INSERT INTO component
		(id,wiki_id,component_key,name,created_by)
		VALUES ($1,$2,'article-infobox','Article information box',$3)`,
		wiki.ComponentID, wiki.ID, wiki.OwnerID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO component_version
		(component_id,version,props_schema_json,renderer_ref,status,created_by,published_at)
		VALUES (
			$1,1,
			'{"type":"object","properties":{"title":{"type":"string"},"language":{"type":"string"},"property_keys":{"type":"array","items":{"type":"string"},"uniqueItems":true}},"additionalProperties":false}'::jsonb,
			'builtin.entity_claim_infobox','published',$2,now()
		)`, wiki.ComponentID, wiki.OwnerID)
	return err
}

func validateRegistration(input RegisterInput) (string, string, string, error) {
	username := strings.ToLower(strings.TrimSpace(input.Username))
	if !usernamePattern.MatchString(username) {
		return "", "", "", &ValidationError{
			Field: "username", Message: "用户名须为 3–32 位小写字母、数字、点、下划线或连字符",
		}
	}
	email := strings.TrimSpace(input.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(address.Address, email) || len(email) > 254 {
		return "", "", "", &ValidationError{Field: "email", Message: "邮箱格式无效"}
	}
	passwordRunes := utf8.RuneCountInString(input.Password)
	if !utf8.ValidString(input.Password) || passwordRunes < passwordMinRunes || passwordRunes > passwordMaxRunes {
		return "", "", "", &ValidationError{
			Field: "password", Message: "密码长度须为 12–128 个字符",
		}
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = username
	}
	if utf8.RuneCountInString(displayName) > 128 {
		return "", "", "", &ValidationError{Field: "display_name", Message: "显示名不得超过 128 个字符"}
	}
	return username, email, displayName, nil
}

func validateProfileUpdate(input UpdateProfileInput) (string, string, string, string, error) {
	if input.ActorID == uuid.Nil {
		return "", "", "", "", ErrAccountNotFound
	}
	email := strings.TrimSpace(input.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || !strings.EqualFold(address.Address, email) || len(email) > 254 {
		return "", "", "", "", &ValidationError{Field: "email", Message: "邮箱格式无效"}
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		return "", "", "", "", &ValidationError{Field: "display_name", Message: "显示名不能为空"}
	}
	if utf8.RuneCountInString(displayName) > 128 {
		return "", "", "", "", &ValidationError{Field: "display_name", Message: "显示名不得超过 128 个字符"}
	}
	wikiName := strings.TrimSpace(input.WikiName)
	if wikiName == "" {
		return "", "", "", "", &ValidationError{Field: "wiki_name", Message: "知识空间名称不能为空"}
	}
	if utf8.RuneCountInString(wikiName) > 128 {
		return "", "", "", "", &ValidationError{Field: "wiki_name", Message: "知识空间名称不得超过 128 个字符"}
	}
	return email, strings.ToLower(email), displayName, wikiName, nil
}

func hashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: generate password salt: %w", err)
	}
	return encodePasswordHash(password, salt), nil
}

func encodePasswordHash(password string, salt []byte) string {
	key := argon2.IDKey(
		[]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLength,
	)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	)
}

func verifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("unsupported password hash")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return false, errors.New("unsupported argon2 version")
	}
	var memory uint32
	var iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return false, errors.New("invalid argon2 parameters")
	}
	if memory != argonMemory || iterations != argonIterations || parallelism != argonParallelism {
		return false, errors.New("unsupported argon2 parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != argonSaltLength {
		return false, errors.New("invalid password salt")
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != argonKeyLength {
		return false, errors.New("invalid password digest")
	}
	actual := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}

func verifyDummyPassword(password string) {
	dummyHashOnce.Do(func() {
		dummyHash = encodePasswordHash("not-a-real-password", []byte("anby-wiki-dummy!"))
	})
	_, _ = verifyPassword(password, dummyHash)
}

func randomToken(size int) (string, error) {
	bytes := make([]byte, size)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("auth: generate random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func tokenHash(token string) [sha256.Size]byte {
	return sha256.Sum256([]byte(token))
}
