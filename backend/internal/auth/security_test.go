package auth

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/anby/wiki/backend/internal/platform/db"
)

func TestTOTPRFC6238AndReplay(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	step, ok := totpStep(secret, "287082", time.Unix(59, 0), -1)
	if !ok || step != 1 {
		t.Fatal("RFC 6238 test vector rejected")
	}
	if _, ok := totpStep(secret, "287082", time.Unix(59, 0), step); ok {
		t.Fatal("replayed TOTP accepted")
	}
	if _, ok := totpStep(secret, "081804", time.Unix(1111111109, 0), -1); !ok {
		t.Fatal("second RFC vector rejected")
	}
	if _, ok := totpStep(secret, "287082", time.Unix(300, 0), -1); ok {
		t.Fatal("expired TOTP accepted")
	}
}

func TestAccountSecurityE2E(t *testing.T) {
	databaseURL := os.Getenv("AUTH_E2E_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set AUTH_E2E_DATABASE_URL for isolated account security tests")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	schemaName := "account_security_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	schemaSQL := pgx.Identifier{schemaName}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schemaSQL); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schemaSQL+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schemaName + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	up, err := os.ReadFile("../../migrations/000001_initial_schema.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(up)); err != nil {
		t.Fatal(err)
	}
	service := NewService(pool, db.NewTxManager(pool), testIDGenerator{}, time.Hour)
	if err := service.ConfigureSecurity(base64.StdEncoding.EncodeToString(make([]byte, 32))); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	service.now = func() time.Time { return now }
	const initialPassword = "account-security-test-password"
	password := initialPassword
	register := func(name string) (Session, Principal) {
		t.Helper()
		session, err := service.Register(ctx, RegisterInput{Username: name, Email: name + "@example.invalid", Password: password})
		if err != nil {
			t.Fatal(err)
		}
		principal, err := service.ResolveSession(ctx, session.Token, uuid.Nil)
		if err != nil {
			t.Fatal(err)
		}
		return session, principal
	}
	owner, principal := register("security_owner")
	otherSession, other := register("security_other")
	if _, err := service.GetMailConfig(ctx, other); !errors.Is(err, ErrSecurityForbidden) {
		t.Fatal("Wiki admin obtained SMTP authority")
	}
	if err := service.SetSystemAdminByUsername(ctx, "security_owner", true); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSystemAdmin(ctx, principal, owner.ActorID, false, password, ""); !errors.Is(err, ErrLastSystemAdmin) {
		t.Fatalf("last administrator check: %v", err)
	}
	cliPrincipal := principal
	cliPrincipal.Method = "cli_token"
	if _, err := service.GetSecurity(ctx, cliPrincipal); !errors.Is(err, ErrSecurityForbidden) {
		t.Fatal("CLI bypassed security session requirement")
	}

	messages, port := startTestSMTP(t)
	config := MailConfig{Enabled: true, Host: "127.0.0.1", Port: port, TLSMode: MailLoopback, FromEmail: "wiki@example.invalid", FromName: "Anby Wiki", PublicURL: "http://localhost:3000"}
	if _, err := service.UpdateMailConfig(ctx, principal, MailConfigInput{Configuration: config, SMTPPassword: "synthetic-smtp-password", Password: password}); err != nil {
		t.Fatal(err)
	}
	saved, err := service.GetMailConfig(ctx, principal)
	if err != nil || !saved.PasswordConfigured {
		t.Fatalf("redacted mail config: %v", err)
	}
	var ciphertext string
	if err := pool.QueryRow(ctx, `SELECT password_ciphertext FROM system_mail_config`).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ciphertext, "synthetic") {
		t.Fatal("SMTP password stored in plaintext")
	}
	if _, err := service.openSecurity(ciphertext, owner.ActorID.String()); err == nil {
		t.Fatal("ciphertext not bound to purpose")
	}

	if _, err := service.MutateSecurity(ctx, principal, SecurityInput{Action: SecurityVerifyEmail}); err != nil {
		t.Fatal(err)
	}
	verifyToken := mailToken(t, messages)
	if _, err := service.MutateSecurity(ctx, principal, SecurityInput{Action: SecurityVerifyEmail}); !errors.Is(err, ErrSecurityCooldown) {
		t.Fatal("verification resend not limited")
	} else {
		var cooldown *SecurityCooldownError
		if !errors.As(err, &cooldown) || cooldown.RetryAfter != time.Minute {
			t.Fatal("verification resend returned an incorrect retry time")
		}
	}
	if err := service.VerifyEmail(ctx, verifyToken, SecurityVerifyEmail); err != nil {
		t.Fatal(err)
	}
	if err := service.VerifyEmail(ctx, verifyToken, SecurityVerifyEmail); !errors.Is(err, ErrSecurityToken) {
		t.Fatal("verification replay accepted")
	}
	state, err := service.GetSecurity(ctx, principal)
	if err != nil || !state.EmailVerified || len(state.Sessions) != 1 || !state.Sessions[0].Current {
		t.Fatalf("security state incorrect: %v", err)
	}
	if err := service.ForgotPassword(ctx, "unknown-account"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-messages:
		t.Fatal("unknown account received recovery mail")
	default:
	}
	code, err := service.CreateCLIAuthorizationCode(ctx, CreateCLIAuthCodeParams{IssuedBy: owner.ActorID, Name: "test", TokenTTLDays: 1})
	if err != nil {
		t.Fatal(err)
	}
	cli, err := service.ExchangeCLIAuthorizationCode(ctx, code.Code)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ForgotPassword(ctx, "security_owner"); err != nil {
		t.Fatal(err)
	}
	resetToken := mailToken(t, messages)
	password = "changed-security-test-password"
	// Concurrent replay must permit exactly one password reset.
	var successes int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := service.ResetPassword(ctx, resetToken, password, "")
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			} else if !errors.Is(err, ErrSecurityToken) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if successes != 1 {
		t.Fatal("concurrent reset token consumption is not atomic")
	}
	if _, err := service.ResolveSession(ctx, owner.Token, uuid.Nil); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("password reset retained browser session")
	}
	if _, err := service.ResolveCLIToken(ctx, cli.Token, uuid.Nil); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("password reset retained CLI token")
	}
	login := func(code string) (Session, Principal) {
		t.Helper()
		session, err := service.Login(ctx, LoginInput{Identifier: "security_owner", Password: password})
		if err != nil {
			t.Fatal(err)
		}
		if session.MFAChallenge != "" {
			session, err = service.CompleteMFALogin(ctx, session.MFAChallenge, code)
			if err != nil {
				t.Fatal(err)
			}
		}
		principal, err := service.ResolveSession(ctx, session.Token, uuid.Nil)
		if err != nil {
			t.Fatal(err)
		}
		return session, principal
	}
	owner, principal = login("")
	setup, err := service.MutateSecurity(ctx, principal, SecurityInput{Action: SecuritySetupMFA, Password: password})
	if err != nil || setup.SetupSecret == "" || !strings.HasPrefix(setup.SetupURI, "otpauth://totp/") {
		t.Fatalf("TOTP setup: %v", err)
	}
	proof := testTOTP(setup.SetupSecret, now)
	enabled, err := service.MutateSecurity(ctx, principal, SecurityInput{Action: SecurityEnableMFA, Password: password, Code: proof})
	if err != nil || len(enabled.RecoveryCodes) != 10 || enabled.Session == nil {
		t.Fatalf("TOTP enable: %v", err)
	}
	if _, err := service.ResolveSession(ctx, owner.Token, uuid.Nil); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("MFA activation retained old session")
	}
	challenge, err := service.Login(ctx, LoginInput{Identifier: "security_owner", Password: password})
	if err != nil || challenge.Token != "" || challenge.MFAChallenge == "" {
		t.Fatal("password-only login bypassed MFA")
	}
	if _, err := service.CompleteMFALogin(ctx, challenge.MFAChallenge, proof); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("replayed TOTP accepted")
	}
	now = now.Add(31 * time.Second)
	validSession, err := service.CompleteMFALogin(ctx, challenge.MFAChallenge, testTOTP(setup.SetupSecret, now))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteMFALogin(ctx, challenge.MFAChallenge, enabled.RecoveryCodes[0]); !errors.Is(err, ErrSecurityToken) {
		t.Fatal("MFA challenge replay accepted")
	}
	principal, err = service.ResolveSession(ctx, validSession.Token, uuid.Nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.MutateSecurity(ctx, principal, SecurityInput{Action: SecurityRevokeSession, Password: password, Code: enabled.RecoveryCodes[0], SessionID: other.CredentialID}); !errors.Is(err, ErrSecurityToken) {
		t.Fatal("foreign session revocation accepted")
	}
	// The failed transaction above must preserve its recovery code.
	_, principal = login(enabled.RecoveryCodes[0])
	challenge, err = service.Login(ctx, LoginInput{Identifier: "security_owner", Password: password})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteMFALogin(ctx, challenge.MFAChallenge, enabled.RecoveryCodes[0]); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("recovery code replay accepted")
	}
	now = now.Add(2 * time.Minute)
	if err := service.ForgotPassword(ctx, "security_owner"); err != nil {
		t.Fatal(err)
	}
	resetToken = mailToken(t, messages)
	if err := service.ResetPassword(ctx, resetToken, "mfa-reset-security-password", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("email-only reset bypassed MFA")
	}
	if err := service.ResetPassword(ctx, resetToken, "mfa-reset-security-password", enabled.RecoveryCodes[1]); err != nil {
		t.Fatal(err)
	}
	password = "mfa-reset-security-password"
	_, principal = login(enabled.RecoveryCodes[2])
	state, err = service.GetSecurity(ctx, principal)
	if err != nil || !state.MFAEnabled || state.RecoveryCodesRemaining != 7 {
		t.Fatalf("reset lost MFA or code consumption: %v", err)
	}
	if _, err := service.MutateSecurity(ctx, principal, SecurityInput{Action: SecurityChangeEmail, Password: password, Code: enabled.RecoveryCodes[3], Email: "new-owner@example.invalid"}); err != nil {
		t.Fatal(err)
	}
	changeToken := mailToken(t, messages)
	state, err = service.GetSecurity(ctx, principal)
	if err != nil || state.Email != "security_owner@example.invalid" {
		t.Fatal("email changed before ownership proof")
	}
	if err := service.VerifyEmail(ctx, changeToken, SecurityChangeEmail); err != nil {
		t.Fatal(err)
	}
	if _, err := service.ResolveSession(ctx, validSession.Token, uuid.Nil); !errors.Is(err, ErrUnauthenticated) {
		t.Fatal("email change did not revoke sessions")
	}
	if err := service.SetSystemAdminByUsername(ctx, "security_other", true); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSystemAdminByUsername(ctx, "security_owner", false); err != nil {
		t.Fatal(err)
	}
	if err := service.SetSystemAdminByUsername(ctx, "security_other", false); !errors.Is(err, ErrLastSystemAdmin) {
		t.Fatal("operator bypassed last admin guard")
	}
	// MFA challenge and account failures remain bounded across fresh challenges.
	challenge, err = service.Login(ctx, LoginInput{Identifier: "security_owner", Password: password})
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := service.CompleteMFALogin(ctx, challenge.MFAChallenge, "invalid"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("wrong proof did not consume its failure budget: %v", err)
		}
	}
	challenge, err = service.Login(ctx, LoginInput{Identifier: "security_owner", Password: password})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CompleteMFALogin(ctx, challenge.MFAChallenge, enabled.RecoveryCodes[4]); !errors.Is(err, ErrSecurityCooldown) {
		t.Fatal("fresh challenge bypassed account security lock")
	}

	upgrade, err := os.ReadFile("../../../scripts/upgrade-account-security.sql")
	if err != nil {
		t.Fatal(err)
	}
	// Reproduce the pre-security schema while retaining the existing accounts,
	// private Wikis and session tokens in this disposable test schema.
	if _, err := pool.Exec(ctx, `DROP TABLE account_security_token,account_recovery_code,system_mail_config,account_security_event;
		ALTER TABLE local_account DROP COLUMN email_verified_at,DROP COLUMN system_admin,
		DROP COLUMN totp_ciphertext,DROP COLUMN totp_pending_ciphertext,DROP COLUMN totp_pending_expires_at,
		DROP COLUMN totp_last_step,DROP COLUMN security_failures,DROP COLUMN security_locked_until;
		ALTER TABLE auth_session DROP COLUMN user_agent,DROP COLUMN last_seen_at,DROP COLUMN mfa_verified`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := pool.Exec(ctx, string(upgrade)); err != nil {
			t.Fatalf("idempotent existing database upgrade: %v", err)
		}
	}
	if _, err := service.ResolveSession(ctx, otherSession.Token, uuid.Nil); err != nil {
		t.Fatalf("upgrade lost the existing session: %v", err)
	}
	if _, err := service.Login(ctx, LoginInput{Identifier: "security_other", Password: initialPassword}); err != nil {
		t.Fatalf("upgrade changed the existing account: %v", err)
	}

	down, err := os.ReadFile("../../migrations/000001_initial_schema.down.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, string(down)); err != nil {
		t.Fatalf("complete down: %v", err)
	}
	if _, err := pool.Exec(ctx, string(up)); err != nil {
		t.Fatalf("second up: %v", err)
	}
}

func testTOTP(secret string, now time.Time) string {
	key, _ := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(now.Unix()/30))
	mac := hmac.New(sha1.New, key)
	_, _ = mac.Write(counter[:])
	digest := mac.Sum(nil)
	offset := digest[len(digest)-1] & 15
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(digest[offset:offset+4])&0x7fffffff)%1000000)
}

func startTestSMTP(t *testing.T) (<-chan string, int) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	messages := make(chan string, 20)
	var workers sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
				reader := bufio.NewReader(conn)
				_, _ = fmt.Fprint(conn, "220 localhost ESMTP\r\n")
				for {
					line, err := reader.ReadString('\n')
					if err != nil {
						return
					}
					switch {
					case strings.HasPrefix(line, "EHLO"), strings.HasPrefix(line, "HELO"):
						_, _ = fmt.Fprint(conn, "250 localhost\r\n")
					case strings.HasPrefix(line, "MAIL"), strings.HasPrefix(line, "RCPT"):
						_, _ = fmt.Fprint(conn, "250 OK\r\n")
					case strings.HasPrefix(line, "DATA"):
						_, _ = fmt.Fprint(conn, "354 End with dot\r\n")
						var data strings.Builder
						for {
							line, err = reader.ReadString('\n')
							if err != nil {
								return
							}
							if line == ".\r\n" {
								break
							}
							data.WriteString(line)
						}
						messages <- data.String()
						_, _ = fmt.Fprint(conn, "250 queued\r\n")
					case strings.HasPrefix(line, "QUIT"):
						_, _ = fmt.Fprint(conn, "221 bye\r\n")
						return
					default:
						_, _ = fmt.Fprint(conn, "500 unsupported\r\n")
					}
				}
			}()
		}
	}()
	t.Cleanup(func() { _ = listener.Close(); <-done; workers.Wait() })
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	number, _ := strconv.Atoi(port)
	return messages, number
}

func mailToken(t *testing.T, messages <-chan string) string {
	t.Helper()
	var message string
	select {
	case message = <-messages:
	case <-time.After(20 * time.Second):
		t.Fatal("SMTP did not receive message")
	}
	parsed, err := mail.ReadMessage(strings.NewReader(message))
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, parsed.Body))
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`#token=([A-Za-z0-9_-]{43})`).FindSubmatch(body)
	if len(match) != 2 {
		t.Fatal("mail missing fragment recovery token")
	}
	return string(match[1])
}
