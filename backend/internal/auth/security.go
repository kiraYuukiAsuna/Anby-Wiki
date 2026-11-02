package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" // RFC 6238 interoperable TOTP.
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var (
	ErrSecurityUnavailable  = errors.New("auth: security configuration unavailable")
	ErrSecurityForbidden    = errors.New("auth: browser session or system administrator required")
	ErrSecurityToken        = errors.New("auth: invalid or expired security token")
	ErrSecurityCooldown     = errors.New("auth: security operation rate limited")
	ErrLastSystemAdmin      = errors.New("auth: cannot remove last system administrator")
	ErrProtectedSystemAdmin = errors.New("auth: cannot remove bootstrap system administrator")
)

type SecurityCooldownError struct {
	RetryAfter time.Duration
}

func (e *SecurityCooldownError) Error() string { return ErrSecurityCooldown.Error() }
func (e *SecurityCooldownError) Unwrap() error { return ErrSecurityCooldown }

const (
	SecurityVerifyEmail    = "verify_email"
	SecurityChangeEmail    = "change_email"
	SecuritySetupMFA       = "setup_mfa"
	SecurityEnableMFA      = "enable_mfa"
	SecurityDisableMFA     = "disable_mfa"
	SecurityRecoveryCodes  = "regenerate_codes"
	SecurityChangePassword = "change_password"
	SecurityRevokeSession  = "revoke_session"
	SecurityRevokeSessions = "revoke_sessions"
	securityResetPassword  = "reset_password"
	securityMFALogin       = "mfa_login"
	securityMailUpdated    = "mail_updated"
	securityMailTested     = "mail_tested"
	securityAdminGranted   = "system_admin_granted"
	securityAdminRevoked   = "system_admin_revoked"
	securityMFARecovered   = "mfa_recovery_code_used"
	securityMFAVerified    = "mfa_login_verified"
	securityAAD            = "anby-wiki/account-security/v1/"
)

type SecurityInput struct {
	Action      string    `json:"action"`
	Password    string    `json:"password"`
	Code        string    `json:"code"`
	Email       string    `json:"email"`
	NewPassword string    `json:"new_password"`
	SessionID   uuid.UUID `json:"session_id"`
}

type SecurityResult struct {
	SetupSecret   string   `json:"setup_secret,omitempty"`
	SetupURI      string   `json:"setup_uri,omitempty"`
	RecoveryCodes []string `json:"recovery_codes,omitempty"`
	SignedOut     bool     `json:"signed_out"`
	Session       *Session `json:"-"`
}

type DeviceSession struct {
	ID         uuid.UUID `json:"id"`
	UserAgent  string    `json:"user_agent"`
	CreatedAt  time.Time `json:"created_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	ExpiresAt  time.Time `json:"expires_at"`
	Current    bool      `json:"current"`
}

type SecurityState struct {
	Email                  string          `json:"email"`
	EmailVerified          bool            `json:"email_verified"`
	MFAEnabled             bool            `json:"mfa_enabled"`
	RecoveryCodesRemaining int             `json:"recovery_codes_remaining"`
	SystemAdmin            bool            `json:"system_admin"`
	MailEnabled            bool            `json:"mail_enabled"`
	Sessions               []DeviceSession `json:"sessions"`
	Events                 []SecurityEvent `json:"events"`
}

type SecurityEvent struct {
	ID             uuid.UUID  `json:"id"`
	ActorID        *uuid.UUID `json:"actor_id"`
	TargetActorID  *uuid.UUID `json:"target_actor_id"`
	ActorUsername  *string    `json:"actor_username"`
	TargetUsername *string    `json:"target_username"`
	Action         string     `json:"action"`
	CreatedAt      time.Time  `json:"created_at"`
}

type securityAccount struct {
	ActorID       uuid.UUID
	Email         string
	Username      string
	PasswordHash  string
	TOTP          string
	PendingTOTP   string
	PendingExpiry *time.Time
	LastStep      int64
	EmailVerified bool
	SystemAdmin   bool
	Failures      int
	LockedUntil   *time.Time
}

// ConfigureSecurity reuses the infrastructure master key with separate,
// purpose- and actor-bound authenticated encryption.
func (s *Service) ConfigureSecurity(encodedKey string) error {
	if strings.TrimSpace(encodedKey) == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedKey))
	if err != nil || len(key) != 32 {
		return ErrSecurityUnavailable
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	s.securityCipher, err = cipher.NewGCM(block)
	return err
}

func (s *Service) sealSecurity(value, purpose string) (string, error) {
	if s.securityCipher == nil {
		return "", ErrSecurityUnavailable
	}
	nonce := make([]byte, s.securityCipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	payload := s.securityCipher.Seal(nonce, nonce, []byte(value), []byte(securityAAD+purpose))
	return base64.StdEncoding.EncodeToString(payload), nil
}

func (s *Service) openSecurity(value, purpose string) (string, error) {
	if s.securityCipher == nil {
		return "", ErrSecurityUnavailable
	}
	payload, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(payload) <= s.securityCipher.NonceSize() {
		return "", ErrSecurityUnavailable
	}
	n := s.securityCipher.NonceSize()
	plaintext, err := s.securityCipher.Open(nil, payload[:n], payload[n:], []byte(securityAAD+purpose))
	if err != nil {
		return "", ErrSecurityUnavailable
	}
	return string(plaintext), nil
}

func requireSecuritySession(principal Principal) error {
	if principal.Method != "session" || principal.ActorType != "human" || principal.CredentialID == uuid.Nil {
		return ErrSecurityForbidden
	}
	return nil
}

func (s *Service) lockSecurityAccount(ctx context.Context, tx pgx.Tx, actorID uuid.UUID) (securityAccount, error) {
	var account securityAccount
	err := tx.QueryRow(ctx, `SELECT la.actor_id,la.email,la.username,la.password_hash,
		la.totp_ciphertext,la.totp_pending_ciphertext,la.totp_pending_expires_at,
		la.totp_last_step,la.email_verified_at IS NOT NULL,la.system_admin,
		la.security_failures,la.security_locked_until
		FROM local_account la JOIN actor a ON a.id=la.actor_id
		WHERE la.actor_id=$1 AND la.status='active' AND a.status='active'
		FOR UPDATE OF la`, actorID).Scan(&account.ActorID, &account.Email, &account.Username,
		&account.PasswordHash, &account.TOTP, &account.PendingTOTP, &account.PendingExpiry,
		&account.LastStep, &account.EmailVerified, &account.SystemAdmin,
		&account.Failures, &account.LockedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return account, ErrInvalidCredentials
	}
	return account, err
}

func (s *Service) securityAccountForPrincipal(ctx context.Context, tx pgx.Tx, principal Principal) (securityAccount, error) {
	if err := requireSecuritySession(principal); err != nil {
		return securityAccount{}, err
	}
	account, err := s.lockSecurityAccount(ctx, tx, principal.ActorID)
	if err != nil {
		return account, err
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth_session WHERE id=$1 AND actor_id=$2
		AND revoked_at IS NULL AND expires_at>now() AND ($3='' OR mfa_verified))`,
		principal.CredentialID, principal.ActorID, account.TOTP).Scan(&active)
	if err != nil {
		return account, err
	}
	if !active {
		return account, ErrUnauthenticated
	}
	return account, nil
}

// Failed proofs are committed so rotating cookies cannot bypass account limits.
func (s *Service) securityTx(ctx context.Context, work func(pgx.Tx) error) error {
	var proofError error
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		err := work(tx)
		if errors.Is(err, ErrInvalidCredentials) || errors.Is(err, ErrSecurityCooldown) {
			proofError = err
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	return proofError
}

func (s *Service) securityProof(ctx context.Context, tx pgx.Tx, account securityAccount, password, code string, checkPassword bool) error {
	now := s.now()
	if account.LockedUntil != nil && account.LockedUntil.After(now) {
		return &SecurityCooldownError{RetryAfter: account.LockedUntil.Sub(now)}
	}
	valid := true
	if checkPassword {
		if len(password) > 512 {
			valid = false
		} else {
			var err error
			valid, err = verifyPassword(password, account.PasswordHash)
			if err != nil {
				return err
			}
		}
	}
	if valid && account.TOTP != "" {
		var err error
		valid, err = s.consumeSecondFactor(ctx, tx, account, code)
		if err != nil {
			return err
		}
	}
	if !valid {
		failures := account.Failures + 1
		if account.LockedUntil != nil && !account.LockedUntil.After(now) {
			failures = 1
		}
		var deadline *time.Time
		if failures >= 5 {
			value := now.Add(10 * time.Minute)
			deadline = &value
		}
		_, err := tx.Exec(ctx, `UPDATE local_account SET security_failures=$2,
			security_locked_until=$3 WHERE actor_id=$1`, account.ActorID, failures, deadline)
		if err != nil {
			return err
		}
		return ErrInvalidCredentials
	}
	_, err := tx.Exec(ctx, `UPDATE local_account SET security_failures=0,
		security_locked_until=NULL WHERE actor_id=$1`, account.ActorID)
	return err
}

func totpStep(secret, code string, now time.Time, lastStep int64) (int64, bool) {
	if len(code) != 6 {
		return 0, false
	}
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		return 0, false
	}
	step := now.Unix() / 30
	for candidate := step - 1; candidate <= step+1; candidate++ {
		if candidate <= lastStep {
			continue
		}
		var counter [8]byte
		binary.BigEndian.PutUint64(counter[:], uint64(candidate))
		mac := hmac.New(sha1.New, key)
		_, _ = mac.Write(counter[:])
		digest := mac.Sum(nil)
		offset := digest[len(digest)-1] & 15
		number := binary.BigEndian.Uint32(digest[offset:offset+4]) & 0x7fffffff
		expected := fmt.Sprintf("%06d", number%1000000)
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return candidate, true
		}
	}
	return 0, false
}

func (s *Service) consumeSecondFactor(ctx context.Context, tx pgx.Tx, account securityAccount, code string) (bool, error) {
	secret, err := s.openSecurity(account.TOTP, account.ActorID.String())
	if err != nil {
		return false, err
	}
	code = strings.TrimSpace(code)
	if step, ok := totpStep(secret, code, s.now(), account.LastStep); ok {
		_, err := tx.Exec(ctx, `UPDATE local_account SET totp_last_step=$2 WHERE actor_id=$1`,
			account.ActorID, step)
		return err == nil, err
	}
	normalized := strings.ToUpper(strings.ReplaceAll(code, "-", ""))
	if len(normalized) != 20 {
		return false, nil
	}
	hash := tokenHash(normalized)
	tag, err := tx.Exec(ctx, `DELETE FROM account_recovery_code WHERE actor_id=$1 AND code_hash=$2`,
		account.ActorID, hash[:])
	if err != nil || tag.RowsAffected() == 0 {
		return false, err
	}
	return true, s.securityAudit(ctx, tx, account.ActorID, account.ActorID, securityMFARecovered)
}

func (s *Service) createSecurityToken(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, purpose, target string, ttl time.Duration) (string, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM account_security_token WHERE actor_id=$1 AND expires_at<$2`,
		actorID, s.now().Add(-24*time.Hour)); err != nil {
		return "", err
	}
	var recent, daily int
	var latest, earliest *time.Time
	err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE created_at>$3),
		count(*),max(created_at),min(created_at)
		FROM account_security_token WHERE actor_id=$1 AND purpose=$2 AND created_at>$4`,
		actorID, purpose, s.now().Add(-time.Minute), s.now().Add(-24*time.Hour)).Scan(&recent, &daily, &latest, &earliest)
	if err != nil {
		return "", err
	}
	if purpose != securityMFALogin && (recent > 0 || daily >= 20) {
		deadline := latest.Add(time.Minute)
		if daily >= 20 && earliest.Add(24*time.Hour).After(deadline) {
			deadline = earliest.Add(24 * time.Hour)
		}
		return "", &SecurityCooldownError{RetryAfter: deadline.Sub(s.now())}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	hash := tokenHash(token)
	if _, err := tx.Exec(ctx, `UPDATE account_security_token SET used_at=$3
		WHERE actor_id=$1 AND purpose=$2 AND used_at IS NULL`, actorID, purpose, s.now()); err != nil {
		return "", err
	}
	_, err = tx.Exec(ctx, `INSERT INTO account_security_token
		(token_hash,actor_id,purpose,target,expires_at,created_at) VALUES ($1,$2,$3,$4,$5,$6)`,
		hash[:], actorID, purpose, target, s.now().Add(ttl), s.now())
	return token, err
}

func (s *Service) securityAudit(ctx context.Context, tx pgx.Tx, actorID, targetID uuid.UUID, action string) error {
	id, err := s.ids.New()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO account_security_event (id,actor_id,target_actor_id,action)
		VALUES ($1,$2,$3,$4)`, id, actorID, targetID, action)
	return err
}

func (s *Service) createSecuritySession(ctx context.Context, tx pgx.Tx, actorID uuid.UUID, mfa bool) (Session, error) {
	var displayName, wikiName string
	var wikiID, namespaceID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT a.display_name,a.wiki_id,w.name,n.id FROM actor a
		JOIN wiki_site w ON w.id=a.wiki_id JOIN namespace n ON n.wiki_id=w.id AND n.namespace_key='main'
		WHERE a.id=$1 AND a.status='active'`, actorID).Scan(&displayName, &wikiID, &wikiName, &namespaceID)
	if err != nil {
		return Session{}, err
	}
	id, session, hash, err := s.newSession(actorID, wikiID, wikiName, namespaceID, displayName)
	if err != nil {
		return Session{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO auth_session(id,token_hash,actor_id,expires_at,mfa_verified)
		VALUES ($1,$2,$3,$4,$5)`, id, hash[:], actorID, session.ExpiresAt, mfa)
	if err != nil {
		return Session{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE local_account SET last_login_at=now() WHERE actor_id=$1`, actorID)
	return session, err
}

func (s *Service) CompleteMFALogin(ctx context.Context, token, code string) (Session, error) {
	var session Session
	err := s.securityTx(ctx, func(tx pgx.Tx) error {
		account, _, err := s.consumeSecurityToken(ctx, tx, token, securityMFALogin)
		if err != nil {
			return err
		}
		if account.TOTP == "" {
			return ErrSecurityToken
		}
		if err := s.securityProof(ctx, tx, account, "", code, false); err != nil {
			return err
		}
		if err := s.markSecurityTokenUsed(ctx, tx, token); err != nil {
			return err
		}
		session, err = s.createSecuritySession(ctx, tx, account.ActorID, true)
		if err != nil {
			return err
		}
		return s.securityAudit(ctx, tx, account.ActorID, account.ActorID, securityMFAVerified)
	})
	return session, err
}

// Account is locked before the token, matching issuance and security mutations.
// Consumption rolls back on storage errors; wrong MFA proofs consume the attempt.
func (s *Service) consumeSecurityToken(ctx context.Context, tx pgx.Tx, token, purpose string) (securityAccount, string, error) {
	if len(token) != 43 {
		return securityAccount{}, "", ErrSecurityToken
	}
	hash := tokenHash(token)
	var actorID uuid.UUID
	err := tx.QueryRow(ctx, `SELECT actor_id FROM account_security_token
		WHERE token_hash=$1 AND purpose=$2`, hash[:], purpose).Scan(&actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return securityAccount{}, "", ErrSecurityToken
	}
	if err != nil {
		return securityAccount{}, "", err
	}
	account, err := s.lockSecurityAccount(ctx, tx, actorID)
	if err != nil {
		return account, "", err
	}
	var target string
	err = tx.QueryRow(ctx, `UPDATE account_security_token SET attempts=attempts+1
		WHERE token_hash=$1 AND purpose=$2 AND used_at IS NULL AND expires_at>$3 AND attempts<5
		RETURNING target`, hash[:], purpose, s.now()).Scan(&target)
	if errors.Is(err, pgx.ErrNoRows) {
		return account, "", ErrSecurityToken
	}
	return account, target, err
}

func (s *Service) markSecurityTokenUsed(ctx context.Context, tx pgx.Tx, token string) error {
	hash := tokenHash(token)
	_, err := tx.Exec(ctx, `UPDATE account_security_token SET used_at=$2 WHERE token_hash=$1`, hash[:], s.now())
	return err
}

func (s *Service) RevokeAccountCredentials(ctx context.Context, tx pgx.Tx, actorID uuid.UUID) error {
	for _, statement := range []string{
		`UPDATE auth_session SET revoked_at=now() WHERE actor_id=$1 AND revoked_at IS NULL`,
		`UPDATE cli_access_token SET revoked_at=now() WHERE actor_id=$1 AND revoked_at IS NULL`,
		`UPDATE cli_authorization_code SET used_at=now() WHERE actor_id=$1 AND used_at IS NULL`,
		`UPDATE cli_access_token SET revoked_at=now() WHERE actor_id IN
			(SELECT id FROM actor WHERE user_id=(SELECT id FROM local_account WHERE actor_id=$1)
				AND id<>$1) AND revoked_at IS NULL`,
		`UPDATE cli_authorization_code SET used_at=now() WHERE actor_id IN
			(SELECT id FROM actor WHERE user_id=(SELECT id FROM local_account WHERE actor_id=$1)
				AND id<>$1) AND used_at IS NULL`,
		`UPDATE account_security_token SET used_at=now() WHERE actor_id=$1 AND used_at IS NULL`,
	} {
		if _, err := tx.Exec(ctx, statement, actorID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) GetSecurity(ctx context.Context, principal Principal) (SecurityState, error) {
	var state SecurityState
	if err := requireSecuritySession(principal); err != nil {
		return state, err
	}
	err := s.pool.QueryRow(ctx, `SELECT email,email_verified_at IS NOT NULL,totp_ciphertext<>'',system_admin,
		(SELECT count(*) FROM account_recovery_code WHERE actor_id=$1),
		EXISTS(SELECT 1 FROM system_mail_config WHERE (configuration->>'enabled')::boolean)
		FROM local_account WHERE actor_id=$1`, principal.ActorID).Scan(&state.Email, &state.EmailVerified,
		&state.MFAEnabled, &state.SystemAdmin, &state.RecoveryCodesRemaining, &state.MailEnabled)
	if err != nil {
		return state, err
	}
	state.Sessions = make([]DeviceSession, 0)
	rows, err := s.pool.Query(ctx, `SELECT id,user_agent,created_at,last_seen_at,expires_at FROM auth_session
		WHERE actor_id=$1 AND revoked_at IS NULL AND expires_at>now() ORDER BY created_at DESC LIMIT 100`, principal.ActorID)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var device DeviceSession
		if err := rows.Scan(&device.ID, &device.UserAgent, &device.CreatedAt, &device.LastSeenAt, &device.ExpiresAt); err != nil {
			rows.Close()
			return state, err
		}
		device.Current = device.ID == principal.CredentialID
		state.Sessions = append(state.Sessions, device)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	state.Events, err = s.listSecurityEvents(ctx, principal.ActorID, 50, 0)
	return state, err
}

func (s *Service) listSecurityEvents(ctx context.Context, actorID uuid.UUID, limit, offset int) ([]SecurityEvent, error) {
	events := make([]SecurityEvent, 0)
	rows, err := s.pool.Query(ctx, `SELECT e.id,e.actor_id,e.target_actor_id,e.action,e.created_at,
		actor.username,target.username FROM account_security_event e
		LEFT JOIN local_account actor ON actor.actor_id=e.actor_id
		LEFT JOIN local_account target ON target.actor_id=e.target_actor_id
		WHERE $1::uuid='00000000-0000-0000-0000-000000000000' OR e.actor_id=$1 OR e.target_actor_id=$1
		ORDER BY e.created_at DESC,e.id DESC LIMIT $2 OFFSET $3`, actorID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var event SecurityEvent
		if err := rows.Scan(&event.ID, &event.ActorID, &event.TargetActorID, &event.Action, &event.CreatedAt,
			&event.ActorUsername, &event.TargetUsername); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *Service) RecordSessionDevice(ctx context.Context, token, userAgent string) error {
	if len(userAgent) > 256 {
		userAgent = userAgent[:256]
	}
	userAgent = strings.ToValidUTF8(userAgent, "")
	hash := tokenHash(token)
	_, err := s.pool.Exec(ctx, `UPDATE auth_session SET user_agent=$2 WHERE token_hash=$1`, hash[:], userAgent)
	return err
}

func (s *Service) MutateSecurity(ctx context.Context, principal Principal, input SecurityInput) (SecurityResult, error) {
	var result SecurityResult
	if err := requireSecuritySession(principal); err != nil {
		return result, err
	}
	err := s.securityTx(ctx, func(tx pgx.Tx) error {
		account, err := s.securityAccountForPrincipal(ctx, tx, principal)
		if err != nil {
			return err
		}
		if input.Action != SecurityVerifyEmail {
			if err := s.securityProof(ctx, tx, account, input.Password, input.Code, true); err != nil {
				return err
			}
		}
		switch input.Action {
		case SecurityVerifyEmail, SecurityChangeEmail:
			email := account.Email
			if input.Action == SecurityChangeEmail {
				parsed, err := mail.ParseAddress(strings.TrimSpace(input.Email))
				if err != nil || parsed.Address != strings.TrimSpace(input.Email) || len(parsed.Address) > 254 {
					return &ValidationError{Field: "email", Message: "请输入有效邮箱"}
				}
				email = parsed.Address
				var exists bool
				if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM local_account
					WHERE normalized_email=$1 AND actor_id<>$2)`, strings.ToLower(email), account.ActorID).Scan(&exists); err != nil {
					return err
				}
				if exists {
					return ErrAccountExists
				}
			}
			token, err := s.createSecurityToken(ctx, tx, account.ActorID, input.Action, email, 30*time.Minute)
			if err != nil {
				return err
			}
			if err := s.sendSecurityEmail(ctx, tx, email, input.Action, token); err != nil {
				return err
			}
		case SecuritySetupMFA:
			if account.TOTP != "" {
				return &ValidationError{Field: "action", Message: "已启用二次验证"}
			}
			raw := make([]byte, 20)
			if _, err := rand.Read(raw); err != nil {
				return err
			}
			result.SetupSecret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
			ciphertext, err := s.sealSecurity(result.SetupSecret, account.ActorID.String())
			if err != nil {
				return err
			}
			_, err = tx.Exec(ctx, `UPDATE local_account SET totp_pending_ciphertext=$2,
				totp_pending_expires_at=$3 WHERE actor_id=$1`, account.ActorID, ciphertext, s.now().Add(10*time.Minute))
			if err != nil {
				return err
			}
			result.SetupURI = "otpauth://totp/" + url.PathEscape("Anby Wiki:"+account.Username) + "?" +
				url.Values{"secret": {result.SetupSecret}, "issuer": {"Anby Wiki"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}.Encode()
		case SecurityEnableMFA:
			if account.TOTP != "" || account.PendingTOTP == "" || account.PendingExpiry == nil || !account.PendingExpiry.After(s.now()) {
				return &ValidationError{Field: "action", Message: "请重新开始绑定验证器"}
			}
			secret, err := s.openSecurity(account.PendingTOTP, account.ActorID.String())
			if err != nil {
				return err
			}
			step, valid := totpStep(secret, strings.TrimSpace(input.Code), s.now(), -1)
			if !valid {
				// Use the same persisted account failure budget for setup confirmation.
				_, err = tx.Exec(ctx, `UPDATE local_account SET security_failures=$2,
					security_locked_until=CASE WHEN $2>=5 THEN $3 ELSE security_locked_until END
					WHERE actor_id=$1`, account.ActorID, account.Failures+1, s.now().Add(10*time.Minute))
				if err != nil {
					return err
				}
				return ErrInvalidCredentials
			}
			_, err = tx.Exec(ctx, `UPDATE local_account SET totp_ciphertext=totp_pending_ciphertext,
				totp_pending_ciphertext='',totp_pending_expires_at=NULL,totp_last_step=$2 WHERE actor_id=$1`, account.ActorID, step)
			if err != nil {
				return err
			}
			result.RecoveryCodes, err = s.replaceRecoveryCodes(ctx, tx, account.ActorID)
			if err != nil {
				return err
			}
			if err := s.RevokeAccountCredentials(ctx, tx, account.ActorID); err != nil {
				return err
			}
			session, err := s.createSecuritySession(ctx, tx, account.ActorID, true)
			if err != nil {
				return err
			}
			result.Session = &session
		case SecurityDisableMFA:
			_, err := tx.Exec(ctx, `UPDATE local_account SET totp_ciphertext='',totp_pending_ciphertext='',
				totp_pending_expires_at=NULL,totp_last_step=-1 WHERE actor_id=$1`, account.ActorID)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `DELETE FROM account_recovery_code WHERE actor_id=$1`, account.ActorID); err != nil {
				return err
			}
			result.SignedOut = true
			if err := s.RevokeAccountCredentials(ctx, tx, account.ActorID); err != nil {
				return err
			}
		case SecurityRecoveryCodes:
			if account.TOTP == "" {
				return &ValidationError{Field: "action", Message: "请先启用二次验证"}
			}
			result.RecoveryCodes, err = s.replaceRecoveryCodes(ctx, tx, account.ActorID)
			if err != nil {
				return err
			}
		case SecurityChangePassword:
			if err := validatePassword(input.NewPassword); err != nil {
				return err
			}
			hash, err := hashPassword(input.NewPassword)
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE local_account SET password_hash=$2,password_changed_at=now(),
				updated_at=now() WHERE actor_id=$1`, account.ActorID, hash); err != nil {
				return err
			}
			result.SignedOut = true
			if err := s.RevokeAccountCredentials(ctx, tx, account.ActorID); err != nil {
				return err
			}
		case SecurityRevokeSession:
			tag, err := tx.Exec(ctx, `UPDATE auth_session SET revoked_at=now() WHERE id=$1 AND actor_id=$2
				AND revoked_at IS NULL`, input.SessionID, account.ActorID)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				return ErrSecurityToken
			}
			result.SignedOut = input.SessionID == principal.CredentialID
		case SecurityRevokeSessions:
			result.SignedOut = true
			if err := s.RevokeAccountCredentials(ctx, tx, account.ActorID); err != nil {
				return err
			}
		default:
			return &ValidationError{Field: "action", Message: "未知账号安全操作"}
		}
		return s.securityAudit(ctx, tx, account.ActorID, account.ActorID, input.Action)
	})
	if err != nil {
		return SecurityResult{}, err
	}
	return result, nil
}

func (s *Service) replaceRecoveryCodes(ctx context.Context, tx pgx.Tx, actorID uuid.UUID) ([]string, error) {
	if _, err := tx.Exec(ctx, `DELETE FROM account_recovery_code WHERE actor_id=$1`, actorID); err != nil {
		return nil, err
	}
	codes := make([]string, 10)
	for i := range codes {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		code := strings.ToUpper(hex.EncodeToString(raw))
		hash := tokenHash(code)
		if _, err := tx.Exec(ctx, `INSERT INTO account_recovery_code(actor_id,code_hash) VALUES($1,$2)`, actorID, hash[:]); err != nil {
			return nil, err
		}
		codes[i] = code[:5] + "-" + code[5:10] + "-" + code[10:15] + "-" + code[15:]
	}
	return codes, nil
}
