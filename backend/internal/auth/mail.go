package auth

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/anby/wiki/backend/internal/platform/db"
)

const (
	MailTLS      = "tls"
	MailSTARTTLS = "starttls"
	MailLoopback = "loopback"

	BootstrapSystemAdminUsername = "admin"
	DefaultSystemPageSize        = 10
	MaxSystemPageSize            = 100
	MaxSystemPage                = 1_000_000
)

// Password is accepted only on writes; it is never stored in configuration JSON.
type MailConfig struct {
	Enabled            bool       `json:"enabled"`
	Host               string     `json:"host"`
	Port               int        `json:"port"`
	TLSMode            string     `json:"tls_mode"`
	Username           string     `json:"username"`
	FromEmail          string     `json:"from_email"`
	FromName           string     `json:"from_name"`
	PublicURL          string     `json:"public_url"`
	PasswordConfigured bool       `json:"password_configured"`
	UpdatedAt          *time.Time `json:"updated_at"`
}

type MailConfigInput struct {
	Configuration MailConfig `json:"configuration"`
	SMTPPassword  string     `json:"smtp_password"`
	Password      string     `json:"password"`
	Code          string     `json:"code"`
}

type SystemAccount struct {
	ActorID              uuid.UUID `json:"actor_id"`
	Username             string    `json:"username"`
	Email                string    `json:"email"`
	SystemAdmin          bool      `json:"system_admin"`
	SystemAdminProtected bool      `json:"system_admin_protected"`
	EmailVerified        bool      `json:"email_verified"`
	MFAEnabled           bool      `json:"mfa_enabled"`
	Status               string    `json:"status"`
}

type SystemStateInput struct {
	Search       string
	AccountsPage int
	EventsPage   int
	PageSize     int
}

type SystemState struct {
	Accounts      []SystemAccount `json:"accounts"`
	Events        []SecurityEvent `json:"events"`
	AccountsPage  int             `json:"accounts_page"`
	EventsPage    int             `json:"events_page"`
	PageSize      int             `json:"page_size"`
	AccountsTotal int64           `json:"accounts_total"`
	EventsTotal   int64           `json:"events_total"`
	MFAEnabled    bool            `json:"mfa_enabled"`
}

func (s *Service) IsSystemAdmin(ctx context.Context, actorID uuid.UUID) (bool, error) {
	var admin bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM local_account la JOIN actor a ON a.id=la.actor_id
		WHERE la.actor_id=$1 AND la.system_admin AND la.status='active' AND a.status='active')`, actorID).Scan(&admin)
	return admin, err
}

func (s *Service) requireSystemAdmin(ctx context.Context, principal Principal) error {
	if err := requireSecuritySession(principal); err != nil {
		return err
	}
	ok, err := s.IsSystemAdmin(ctx, principal.ActorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrSecurityForbidden
	}
	return nil
}

func (s *Service) mailConfiguration(ctx context.Context, querier db.Querier) (MailConfig, string, error) {
	config := MailConfig{Port: 587, TLSMode: MailSTARTTLS, FromName: "Anby Wiki"}
	var raw []byte
	var ciphertext string
	var updated time.Time
	err := querier.QueryRow(ctx, `SELECT configuration,password_ciphertext,updated_at FROM system_mail_config
		WHERE singleton`).Scan(&raw, &ciphertext, &updated)
	if errors.Is(err, pgx.ErrNoRows) {
		return config, "", nil
	}
	if err != nil {
		return config, "", err
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return config, "", err
	}
	config.PasswordConfigured = ciphertext != ""
	config.UpdatedAt = &updated
	return config, ciphertext, nil
}

func (s *Service) GetMailConfig(ctx context.Context, principal Principal) (MailConfig, error) {
	if err := s.requireSystemAdmin(ctx, principal); err != nil {
		return MailConfig{}, err
	}
	config, _, err := s.mailConfiguration(ctx, s.pool)
	return config, err
}

func validateMailConfig(config MailConfig) error {
	invalid := &ValidationError{Field: "configuration", Message: "请检查 SMTP 地址、端口、发件邮箱和站点地址"}
	if len(config.Host) > 253 || config.Port < 1 || config.Port > 65535 || strings.ContainsAny(config.Host, "/\\ \r\n:") ||
		config.Host == "" || len(config.Username) > 254 || len(config.FromName) > 128 || strings.ContainsAny(config.FromName+config.Username, "\r\n") {
		return invalid
	}
	address, err := mail.ParseAddress(config.FromEmail)
	if err != nil || address.Address != config.FromEmail || len(config.FromEmail) > 254 {
		return invalid
	}
	site, err := url.Parse(config.PublicURL)
	if err != nil || site.Host == "" || site.User != nil || site.RawQuery != "" || site.Fragment != "" ||
		(site.Path != "" && site.Path != "/") || len(config.PublicURL) > 2048 {
		return invalid
	}
	if site.Scheme != "https" && !(site.Scheme == "http" && loopbackHost(site.Hostname())) {
		return invalid
	}
	switch config.TLSMode {
	case MailTLS, MailSTARTTLS:
	case MailLoopback:
		if !loopbackHost(config.Host) {
			return &ValidationError{Field: "tls_mode", Message: "明文 SMTP 仅允许本机测试"}
		}
	default:
		return invalid
	}
	return nil
}

func loopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Service) UpdateMailConfig(ctx context.Context, principal Principal, input MailConfigInput) (MailConfig, error) {
	if err := s.requireSystemAdmin(ctx, principal); err != nil {
		return MailConfig{}, err
	}
	if err := validateMailConfig(input.Configuration); err != nil {
		return MailConfig{}, err
	}
	if len(input.SMTPPassword) > 2048 {
		return MailConfig{}, &ValidationError{Field: "smtp_password", Message: "SMTP 密码过长"}
	}
	err := s.securityTx(ctx, func(tx pgx.Tx) error {
		account, err := s.securityAccountForPrincipal(ctx, tx, principal)
		if err != nil {
			return err
		}
		if !account.SystemAdmin {
			return ErrSecurityForbidden
		}
		if err := s.securityProof(ctx, tx, account, input.Password, input.Code, true); err != nil {
			return err
		}
		_, ciphertext, err := s.mailConfiguration(ctx, tx)
		if err != nil {
			return err
		}
		if input.SMTPPassword != "" {
			ciphertext, err = s.sealSecurity(input.SMTPPassword, "smtp")
			if err != nil {
				return err
			}
		}
		if input.Configuration.Username != "" && ciphertext == "" {
			return &ValidationError{Field: "smtp_password", Message: "请填写 SMTP 密码"}
		}
		config := input.Configuration
		config.PasswordConfigured = false
		config.UpdatedAt = nil
		config.PublicURL = strings.TrimRight(config.PublicURL, "/")
		raw, err := json.Marshal(config)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO system_mail_config(singleton,configuration,password_ciphertext)
			VALUES (true,$1,$2) ON CONFLICT(singleton) DO UPDATE SET
			configuration=EXCLUDED.configuration,password_ciphertext=EXCLUDED.password_ciphertext,updated_at=now()`, raw, ciphertext); err != nil {
			return err
		}
		return s.securityAudit(ctx, tx, principal.ActorID, principal.ActorID, securityMailUpdated)
	})
	if err != nil {
		return MailConfig{}, err
	}
	return s.GetMailConfig(ctx, principal)
}

func (s *Service) TestMail(ctx context.Context, principal Principal, email string) error {
	if err := s.requireSystemAdmin(ctx, principal); err != nil {
		return err
	}
	email = strings.TrimSpace(email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return &ValidationError{Field: "email", Message: "请输入有效收件邮箱"}
	}
	return s.securityTx(ctx, func(tx pgx.Tx) error {
		account, err := s.securityAccountForPrincipal(ctx, tx, principal)
		if err != nil {
			return err
		}
		if !account.SystemAdmin {
			return ErrSecurityForbidden
		}
		config, ciphertext, err := s.mailConfiguration(ctx, tx)
		if err != nil {
			return err
		}
		if err := s.deliverMail(ctx, config, ciphertext, email, "邮件配置测试", "Anby Wiki 邮件发送配置测试成功。"); err != nil {
			return err
		}
		return s.securityAudit(ctx, tx, principal.ActorID, principal.ActorID, securityMailTested)
	})
}

func (s *Service) sendSecurityEmail(ctx context.Context, querier db.Querier, email, purpose, token string) error {
	config, ciphertext, err := s.mailConfiguration(ctx, querier)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return ErrSecurityUnavailable
	}
	path := "/verify-email"
	subject := "验证邮箱"
	body := "请打开以下链接确认邮箱所有权。链接 30 分钟内有效，且只能使用一次。"
	if purpose == securityResetPassword {
		path = "/reset-password"
		subject = "重置密码"
		body = "请打开以下链接重置密码。链接 15 分钟内有效，且只能使用一次。二次验证仍然有效。"
	}
	// Fragment tokens are omitted from server access logs and referrer headers.
	link := config.PublicURL + path + "#token=" + url.QueryEscape(token) + "&purpose=" + url.QueryEscape(purpose)
	body += "\n\n" + link + "\n\n如果你没有发起此操作，请忽略此邮件。"
	return s.deliverMail(ctx, config, ciphertext, email, subject, body)
}

func (s *Service) deliverMail(ctx context.Context, config MailConfig, ciphertext, recipient, subject, body string) error {
	if err := validateMailConfig(config); err != nil {
		return ErrSecurityUnavailable
	}
	password := ""
	if ciphertext != "" {
		var err error
		password, err = s.openSecurity(ciphertext, "smtp")
		if err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	address := net.JoinHostPort(config.Host, strconv.Itoa(config.Port))
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return ErrSecurityUnavailable
	}
	defer conn.Close()
	rawConn := conn
	stop := context.AfterFunc(ctx, func() { _ = rawConn.Close() })
	defer stop()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	tlsConfig := &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12}
	if config.TLSMode == MailTLS {
		secure := tls.Client(conn, tlsConfig)
		if err := secure.HandshakeContext(ctx); err != nil {
			return ErrSecurityUnavailable
		}
		conn = secure
	}
	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		return ErrSecurityUnavailable
	}
	defer client.Close()
	if config.TLSMode == MailSTARTTLS {
		if ok, _ := client.Extension("STARTTLS"); !ok {
			return ErrSecurityUnavailable
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return ErrSecurityUnavailable
		}
	}
	if config.Username != "" {
		if err := client.Auth(smtp.PlainAuth("", config.Username, password, config.Host)); err != nil {
			return ErrSecurityUnavailable
		}
	}
	if err := client.Mail(config.FromEmail); err != nil {
		return ErrSecurityUnavailable
	}
	if err := client.Rcpt(recipient); err != nil {
		return ErrSecurityUnavailable
	}
	writer, err := client.Data()
	if err != nil {
		return ErrSecurityUnavailable
	}
	from := (&mail.Address{Name: config.FromName, Address: config.FromEmail}).String()
	message := "From: " + from + "\r\nTo: " + recipient + "\r\nSubject: =?UTF-8?B?" +
		base64.StdEncoding.EncodeToString([]byte(subject)) + "?=\r\nMIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: base64\r\n\r\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	for len(encoded) > 76 {
		message += encoded[:76] + "\r\n"
		encoded = encoded[76:]
	}
	message += encoded + "\r\n"
	if _, err := writer.Write([]byte(message)); err != nil {
		return ErrSecurityUnavailable
	}
	if err := writer.Close(); err != nil {
		return ErrSecurityUnavailable
	}
	if err := client.Quit(); err != nil {
		return ErrSecurityUnavailable
	}
	return nil
}

// Enumeration-safe: only a verified, active mailbox receives a reset link.
func (s *Service) ForgotPassword(ctx context.Context, identifier string) error {
	config, _, err := s.mailConfiguration(ctx, s.pool)
	if err != nil {
		return err
	}
	if !config.Enabled {
		return ErrSecurityUnavailable
	}
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	var actorID uuid.UUID
	err = s.pool.QueryRow(ctx, `SELECT la.actor_id FROM local_account la JOIN actor a ON a.id=la.actor_id
		WHERE (normalized_username=$1 OR normalized_email=$1) AND email_verified_at IS NOT NULL
		AND la.status='active' AND a.status='active'`, identifier).Scan(&actorID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	err = s.txm.InTx(ctx, func(tx pgx.Tx) error {
		account, err := s.lockSecurityAccount(ctx, tx, actorID)
		if err != nil {
			return err
		}
		if !account.EmailVerified {
			return nil
		}
		token, err := s.createSecurityToken(ctx, tx, actorID, securityResetPassword, account.Email, 15*time.Minute)
		if err != nil {
			return err
		}
		if err := s.sendSecurityEmail(ctx, tx, account.Email, securityResetPassword, token); err != nil {
			return err
		}
		return s.securityAudit(ctx, tx, actorID, actorID, securityResetPassword+"_requested")
	})
	// Do not expose recipient-specific cooldown or transport/account errors.
	if err != nil {
		return nil
	}
	return nil
}

func (s *Service) VerifyEmail(ctx context.Context, token, purpose string) error {
	if purpose != SecurityVerifyEmail && purpose != SecurityChangeEmail {
		return ErrSecurityToken
	}
	err := s.txm.InTx(ctx, func(tx pgx.Tx) error {
		account, target, err := s.consumeSecurityToken(ctx, tx, token, purpose)
		if err != nil {
			return err
		}
		if purpose == SecurityVerifyEmail && !strings.EqualFold(account.Email, target) {
			return ErrSecurityToken
		}
		if _, err := tx.Exec(ctx, `UPDATE local_account SET email=$2,normalized_email=$3,
			email_verified_at=now(),updated_at=now() WHERE actor_id=$1`, account.ActorID, target, strings.ToLower(target)); err != nil {
			return err
		}
		if err := s.markSecurityTokenUsed(ctx, tx, token); err != nil {
			return err
		}
		if purpose == SecurityChangeEmail {
			if err := s.RevokeAccountCredentials(ctx, tx, account.ActorID); err != nil {
				return err
			}
		}
		return s.securityAudit(ctx, tx, account.ActorID, account.ActorID, purpose+"_confirmed")
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrAccountExists
	}
	return err
}

func (s *Service) ResetPassword(ctx context.Context, token, password, code string) error {
	if err := validatePassword(password); err != nil {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	return s.securityTx(ctx, func(tx pgx.Tx) error {
		account, target, err := s.consumeSecurityToken(ctx, tx, token, securityResetPassword)
		if err != nil {
			return err
		}
		if !account.EmailVerified || !strings.EqualFold(account.Email, target) {
			return ErrSecurityToken
		}
		if err := s.securityProof(ctx, tx, account, "", code, false); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE local_account SET password_hash=$2,password_changed_at=now(),
			updated_at=now() WHERE actor_id=$1`, account.ActorID, hash); err != nil {
			return err
		}
		if err := s.RevokeAccountCredentials(ctx, tx, account.ActorID); err != nil {
			return err
		}
		return s.securityAudit(ctx, tx, account.ActorID, account.ActorID, securityResetPassword)
	})
}

func (s *Service) GetSystemState(ctx context.Context, principal Principal, input SystemStateInput) (SystemState, error) {
	state := SystemState{Accounts: make([]SystemAccount, 0), Events: make([]SecurityEvent, 0)}
	if err := s.requireSystemAdmin(ctx, principal); err != nil {
		return state, err
	}
	if input.AccountsPage == 0 {
		input.AccountsPage = 1
	}
	if input.EventsPage == 0 {
		input.EventsPage = 1
	}
	if input.PageSize == 0 {
		input.PageSize = DefaultSystemPageSize
	}
	if input.AccountsPage < 1 || input.AccountsPage > MaxSystemPage ||
		input.EventsPage < 1 || input.EventsPage > MaxSystemPage ||
		input.PageSize < 1 || input.PageSize > MaxSystemPageSize {
		return state, &ValidationError{Field: "pagination", Message: "请检查页码和每页条数"}
	}
	input.Search = strings.ToLower(strings.TrimSpace(input.Search))
	if len(input.Search) > 254 {
		return state, &ValidationError{Field: "search", Message: "搜索内容不能超过 254 字节"}
	}
	err := s.pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM local_account WHERE $2='' OR strpos(normalized_username,$2)>0 OR strpos(lower(email),$2)>0),
		(SELECT count(*) FROM account_security_event),totp_ciphertext<>''
		FROM local_account WHERE actor_id=$1`, principal.ActorID, input.Search).
		Scan(&state.AccountsTotal, &state.EventsTotal, &state.MFAEnabled)
	if err != nil {
		return state, err
	}
	state.PageSize = input.PageSize
	state.AccountsPage = min(input.AccountsPage, max(1, int((state.AccountsTotal+int64(input.PageSize)-1)/int64(input.PageSize))))
	state.EventsPage = min(input.EventsPage, max(1, int((state.EventsTotal+int64(input.PageSize)-1)/int64(input.PageSize))))
	rows, err := s.pool.Query(ctx, `SELECT actor_id,username,email,system_admin,email_verified_at IS NOT NULL,
		totp_ciphertext<>'',status,normalized_username=$1 FROM local_account
		WHERE $2='' OR strpos(normalized_username,$2)>0 OR strpos(lower(email),$2)>0
		ORDER BY system_admin DESC,created_at,actor_id LIMIT $3 OFFSET $4`,
		BootstrapSystemAdminUsername, input.Search, input.PageSize, (state.AccountsPage-1)*input.PageSize)
	if err != nil {
		return state, err
	}
	for rows.Next() {
		var account SystemAccount
		if err := rows.Scan(&account.ActorID, &account.Username, &account.Email, &account.SystemAdmin, &account.EmailVerified,
			&account.MFAEnabled, &account.Status, &account.SystemAdminProtected); err != nil {
			rows.Close()
			return state, err
		}
		state.Accounts = append(state.Accounts, account)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return state, err
	}
	state.Events, err = s.listSecurityEvents(ctx, uuid.Nil, input.PageSize, (state.EventsPage-1)*input.PageSize)
	return state, err
}

func (s *Service) SetSystemAdmin(ctx context.Context, principal Principal, targetID uuid.UUID, grant bool, password, code string) error {
	if err := s.requireSystemAdmin(ctx, principal); err != nil {
		return err
	}
	return s.securityTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('system_administrators',0))`); err != nil {
			return err
		}
		account, err := s.securityAccountForPrincipal(ctx, tx, principal)
		if err != nil {
			return err
		}
		if !account.SystemAdmin {
			return ErrSecurityForbidden
		}
		if err := s.securityProof(ctx, tx, account, password, code, true); err != nil {
			return err
		}
		return s.setSystemAdmin(ctx, tx, principal.ActorID, targetID, grant)
	})
}

func (s *Service) setSystemAdmin(ctx context.Context, tx pgx.Tx, actorID, targetID uuid.UUID, grant bool) error {
	target, err := s.lockSecurityAccount(ctx, tx, targetID)
	if err != nil {
		return err
	}
	if !grant && target.Username == BootstrapSystemAdminUsername {
		return ErrProtectedSystemAdmin
	}
	if target.SystemAdmin == grant {
		return nil
	}
	if !grant {
		var count int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM local_account la JOIN actor a ON a.id=la.actor_id
			WHERE system_admin AND la.status='active' AND a.status='active'`).Scan(&count); err != nil {
			return err
		}
		if count <= 1 {
			return ErrLastSystemAdmin
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE local_account SET system_admin=$2,updated_at=now()
		WHERE actor_id=$1`, targetID, grant); err != nil {
		return err
	}
	action := securityAdminGranted
	if !grant {
		action = securityAdminRevoked
	}
	return s.securityAudit(ctx, tx, actorID, targetID, action)
}

// Operator-only bootstrap; this method is deliberately absent from HTTP routes.
func (s *Service) SetSystemAdminByUsername(ctx context.Context, username string, grant bool) error {
	return s.txm.InTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('system_administrators',0))`); err != nil {
			return err
		}
		var actorID uuid.UUID
		err := tx.QueryRow(ctx, `SELECT actor_id FROM local_account WHERE normalized_username=$1`, strings.ToLower(strings.TrimSpace(username))).Scan(&actorID)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		var operatorID uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT id FROM actor WHERE actor_type='system' ORDER BY id LIMIT 1`).Scan(&operatorID); err != nil {
			return err
		}
		return s.setSystemAdmin(ctx, tx, operatorID, actorID, grant)
	})
}

func validatePassword(password string) error {
	if count := len([]rune(password)); count < passwordMinRunes || count > passwordMaxRunes {
		return &ValidationError{Field: "password", Message: fmt.Sprintf("密码长度须为 %d–%d 个字符", passwordMinRunes, passwordMaxRunes)}
	}
	return nil
}
