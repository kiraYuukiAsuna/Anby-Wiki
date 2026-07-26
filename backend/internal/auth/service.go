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

// Session contains the opaque cookie value. Only its SHA-256 hash is stored.
type Session struct {
	Token       string
	ActorID     uuid.UUID
	DisplayName string
	ExpiresAt   time.Time
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

// Register creates a local account, its Actor, its initial wiki role and the
// first session in one transaction. The first local account is the site admin;
// later accounts receive the editor role.
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
	sessionID, session, hash, err := s.newSession(actorID, displayName)
	if err != nil {
		return Session{}, err
	}

	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		// Serialize first-account selection so exactly one registrant receives
		// the bootstrap admin role.
		if _, err := tx.Exec(ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended('anby-wiki:first-local-account', 0))`); err != nil {
			return err
		}
		var firstAccount bool
		if err := tx.QueryRow(ctx, `SELECT NOT EXISTS (SELECT 1 FROM local_account)`).
			Scan(&firstAccount); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO actor
			(id,actor_type,display_name,status) VALUES ($1,'human',$2,'active')`,
			actorID, displayName); err != nil {
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
		roleKey := "editor"
		if firstAccount {
			roleKey = "admin"
		}
		tag, err := tx.Exec(ctx, `INSERT INTO actor_role (actor_id,role_id,wiki_id)
			SELECT $1,r.id,w.id FROM role r CROSS JOIN wiki_site w
			WHERE r.role_key=$2 AND w.site_key='default'`, actorID, roleKey)
		if err != nil {
			return err
		}
		if tag.RowsAffected() != 1 {
			return fmt.Errorf("auth: default wiki or role %q is missing", roleKey)
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

	var accountID, actorID uuid.UUID
	var passwordHash, displayName, accountStatus, actorStatus string
	err := s.pool.QueryRow(ctx, `SELECT la.id,la.actor_id,la.password_hash,a.display_name,la.status,a.status
		FROM local_account la JOIN actor a ON a.id=la.actor_id
		WHERE la.normalized_username=$1 OR la.normalized_email=$1`,
		identifier).Scan(
		&accountID, &actorID, &passwordHash, &displayName, &accountStatus, &actorStatus,
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

	sessionID, session, hash, err := s.newSession(actorID, displayName)
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
	sessionID, session, hash, err := s.newSession(uuid.Nil, identity.DisplayName)
	if err != nil {
		return Session{}, err
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			identity.Issuer+"\x1f"+identity.Subject); err != nil {
			return err
		}
		var actorStatus string
		err := tx.QueryRow(ctx, `SELECT ei.actor_id,a.status,a.display_name FROM external_identity ei
			JOIN actor a ON a.id=ei.actor_id
			WHERE ei.issuer=$1 AND ei.subject=$2`,
			identity.Issuer, identity.Subject).Scan(
			&session.ActorID, &actorStatus, &session.DisplayName,
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
			if _, err := tx.Exec(ctx, `INSERT INTO actor
				(id,actor_type,display_name,status) VALUES ($1,'human',$2,'active')`,
				session.ActorID, identity.DisplayName); err != nil {
				return err
			}
			session.DisplayName = identity.DisplayName
			if _, err := tx.Exec(ctx, `INSERT INTO external_identity
				(issuer,subject,actor_id) VALUES ($1,$2,$3)`,
				identity.Issuer, identity.Subject, session.ActorID); err != nil {
				return err
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
func (s *Service) ResolveSession(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	hash := tokenHash(token)
	var principal Principal
	err := s.pool.QueryRow(ctx, `SELECT a.id,a.actor_type,a.display_name FROM auth_session s
		JOIN actor a ON a.id=s.actor_id
		LEFT JOIN local_account la ON la.actor_id=a.id
		WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>now()
			AND a.status='active' AND (la.id IS NULL OR la.status='active')`, hash[:]).Scan(
		&principal.ActorID, &principal.ActorType, &principal.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: resolve session: %w", err)
	}
	principal.Method = "session"
	return principal, nil
}

// ResolveDevActor validates an explicitly supplied development/test Actor.
func (s *Service) ResolveDevActor(ctx context.Context, actorID uuid.UUID) (Principal, error) {
	var principal Principal
	err := s.pool.QueryRow(ctx, `SELECT id,actor_type,display_name FROM actor
		WHERE id=$1 AND status='active'`, actorID).Scan(
		&principal.ActorID, &principal.ActorType, &principal.DisplayName,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, fmt.Errorf("auth: resolve development actor: %w", err)
	}
	principal.Method = "development_header"
	return principal, nil
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

func (s *Service) newSession(actorID uuid.UUID, displayName string) (uuid.UUID, Session, [sha256.Size]byte, error) {
	token, err := randomToken(32)
	if err != nil {
		return uuid.Nil, Session{}, [sha256.Size]byte{}, err
	}
	sessionID, err := s.ids.New()
	if err != nil {
		return uuid.Nil, Session{}, [sha256.Size]byte{}, fmt.Errorf("auth: generate session id: %w", err)
	}
	session := Session{
		Token:       token,
		ActorID:     actorID,
		DisplayName: displayName,
		ExpiresAt:   s.now().Add(s.sessionTTL),
	}
	return sessionID, session, tokenHash(token), nil
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
