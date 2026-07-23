// Package config 从环境变量加载并校验应用配置。
package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config 应用配置，全部由环境变量注入。
type Config struct {
	// Port HTTP 监听端口，默认 8080。
	Port int `env:"PORT" envDefault:"8080"`
	// DatabaseURL PostgreSQL 连接串，必填。
	DatabaseURL string `env:"DATABASE_URL"`
	// RedisURL Redis 连接串，必填。
	RedisURL string `env:"REDIS_URL"`
	// S3Endpoint 对象存储端点，必填。
	S3Endpoint string `env:"S3_ENDPOINT"`
	// S3Bucket 对象存储桶名，必填。
	S3Bucket string `env:"S3_BUCKET"`
	// S3AccessKey 对象存储访问密钥，必填。
	S3AccessKey string `env:"S3_ACCESS_KEY"`
	// S3SecretKey 对象存储私有密钥，必填。
	S3SecretKey string `env:"S3_SECRET_KEY"`
	// S3Region 对象存储签名区域；MinIO 默认使用 us-east-1。
	S3Region string `env:"S3_REGION" envDefault:"us-east-1"`
	// LogLevel 日志级别（debug/info/warn/error），默认 info。
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	// Env 运行环境（development/staging/production），默认 development。
	Env string `env:"ENV" envDefault:"development"`
	// SearchBackend selects postgres staging/fallback or Meilisearch.
	SearchBackend string `env:"SEARCH_BACKEND" envDefault:"postgres"`
	MeiliURL      string `env:"MEILI_URL" envDefault:"http://localhost:7700"`
	// MeiliAPIKey is secret material and must never be logged.
	MeiliAPIKey  string        `env:"MEILI_API_KEY"`
	MeiliIndex   string        `env:"MEILI_INDEX" envDefault:"anby_pages"`
	MeiliTimeout time.Duration `env:"MEILI_TIMEOUT" envDefault:"15s"`
	// WorkerMetricsAddr 是 Worker 独立指标监听地址；空字符串可显式关闭。
	WorkerMetricsAddr string `env:"WORKER_METRICS_ADDR" envDefault:":9091"`
	// ObservabilityDBInterval 控制 Worker 从数据库刷新低侵入指标的周期。
	ObservabilityDBInterval time.Duration `env:"OBSERVABILITY_DB_INTERVAL" envDefault:"30s"`
	// OTelEnabled 显式启用 OTLP/gRPC trace export。
	OTelEnabled    bool    `env:"OTEL_ENABLED" envDefault:"false"`
	OTLPEndpoint   string  `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTLPInsecure   bool    `env:"OTEL_EXPORTER_OTLP_INSECURE" envDefault:"false"`
	OTelSampleRate float64 `env:"OTEL_TRACE_SAMPLE_RATE" envDefault:"1"`
	// AIImportEnabled 显式启用常驻 Worker 的来源导入消费；默认关闭，避免
	// 未配置模型凭据时误消费任务。
	AIImportEnabled bool `env:"AI_IMPORT_ENABLED" envDefault:"false"`
	// AIProvider 是 Gateway 内的供应商键。当前内置 openai-compatible Adapter。
	AIProvider string `env:"AI_PROVIDER" envDefault:"openai-compatible"`
	// AIBaseURL 是 OpenAI-compatible API 根地址（例如 https://host/v1）。
	AIBaseURL string `env:"AI_BASE_URL"`
	// AIAPIKey 只从进程环境注入；禁止写入配置文件或日志。
	AIAPIKey string `env:"AI_API_KEY"`
	// AIModel 是导入抽取使用的模型 ID。
	AIModel string `env:"AI_MODEL"`
	// OIDCEnabled controls generic OpenID Connect authentication.
	OIDCEnabled      bool   `env:"OIDC_ENABLED" envDefault:"false"`
	OIDCIssuerURL    string `env:"OIDC_ISSUER_URL"`
	OIDCClientID     string `env:"OIDC_CLIENT_ID"`
	OIDCClientSecret string `env:"OIDC_CLIENT_SECRET"`
	OIDCRedirectURL  string `env:"OIDC_REDIRECT_URL"`
	OIDCScopes       string `env:"OIDC_SCOPES" envDefault:"profile email"`
	// AuthDevHeaderEnabled permits X-Actor-ID only in development/test.
	AuthDevHeaderEnabled  bool          `env:"AUTH_DEV_HEADER_ENABLED" envDefault:"false"`
	SessionCookieName     string        `env:"SESSION_COOKIE_NAME" envDefault:"anby_session"`
	SessionCookieSecure   bool          `env:"SESSION_COOKIE_SECURE" envDefault:"true"`
	SessionTTL            time.Duration `env:"SESSION_TTL" envDefault:"24h"`
	AuthPostLoginRedirect string        `env:"AUTH_POST_LOGIN_REDIRECT" envDefault:"/"`
	// TrustedOrigins 是允许携带 session cookie 发起写请求的精确 HTTPS origin。
	TrustedOrigins []string `env:"TRUSTED_ORIGINS" envSeparator:","`
}

// Load 从进程环境变量加载配置并校验必填项。
// 缺失必填项时返回聚合错误，错误信息包含所有缺失字段名；
// 此时仍返回已解析的配置（含默认值），调用方可在降级模式下继续使用。
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("config: 解析环境变量失败: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// validate 校验必填字段，返回带字段名的聚合错误。
func (c Config) validate() error {
	var missing []string
	required := map[string]string{
		"DATABASE_URL":  c.DatabaseURL,
		"REDIS_URL":     c.RedisURL,
		"S3_ENDPOINT":   c.S3Endpoint,
		"S3_BUCKET":     c.S3Bucket,
		"S3_ACCESS_KEY": c.S3AccessKey,
		"S3_SECRET_KEY": c.S3SecretKey,
	}
	for name, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: 缺失必填环境变量: %s", strings.Join(missing, ", "))
	}
	switch c.SearchBackend {
	case "postgres":
	case "meilisearch":
		if strings.TrimSpace(c.MeiliURL) == "" {
			return fmt.Errorf("config: SEARCH_BACKEND=meilisearch 时缺失环境变量: MEILI_URL")
		}
		if strings.TrimSpace(c.MeiliIndex) == "" {
			return fmt.Errorf("config: SEARCH_BACKEND=meilisearch 时 MEILI_INDEX 不能为空")
		}
		if c.MeiliTimeout <= 0 {
			return fmt.Errorf("config: MEILI_TIMEOUT 必须大于 0")
		}
		if err := validateServiceURL(c.MeiliURL); err != nil {
			return fmt.Errorf("config: MEILI_URL 非法: %w", err)
		}
	default:
		return fmt.Errorf("config: 不支持的 SEARCH_BACKEND: %s", c.SearchBackend)
	}
	if c.AIImportEnabled {
		var aiMissing []string
		for name, value := range map[string]string{
			"AI_BASE_URL": c.AIBaseURL,
			"AI_API_KEY":  c.AIAPIKey,
			"AI_MODEL":    c.AIModel,
		} {
			if strings.TrimSpace(value) == "" {
				aiMissing = append(aiMissing, name)
			}
		}
		if len(aiMissing) > 0 {
			return fmt.Errorf("config: AI_IMPORT_ENABLED=true 时缺失环境变量: %s", strings.Join(aiMissing, ", "))
		}
		if c.AIProvider != "openai-compatible" {
			return fmt.Errorf("config: 不支持的 AI_PROVIDER: %s", c.AIProvider)
		}
	}
	if c.Env == "production" {
		if c.SearchBackend != "meilisearch" {
			return fmt.Errorf("config: production 要求 SEARCH_BACKEND=meilisearch")
		}
		if strings.TrimSpace(c.MeiliAPIKey) == "" {
			return fmt.Errorf("config: production 要求 MEILI_API_KEY 非空")
		}
		if c.AuthDevHeaderEnabled {
			return fmt.Errorf("config: production 严禁 AUTH_DEV_HEADER_ENABLED=true")
		}
		if !c.SessionCookieSecure {
			return fmt.Errorf("config: production 要求 SESSION_COOKIE_SECURE=true")
		}
		if !c.OIDCEnabled {
			return fmt.Errorf("config: production 要求 OIDC_ENABLED=true")
		}
		if len(c.TrustedOrigins) == 0 {
			return fmt.Errorf("config: production 要求 TRUSTED_ORIGINS")
		}
		if weakSecret(c.S3AccessKey) || weakSecret(c.S3SecretKey) {
			return fmt.Errorf("config: production 拒绝 S3 弱默认 Secret")
		}
	}
	for _, origin := range c.TrustedOrigins {
		if err := validateTrustedOrigin(origin, c.Env == "production"); err != nil {
			return fmt.Errorf("config: TRUSTED_ORIGINS 包含非法 origin %q: %w", origin, err)
		}
	}
	if c.OIDCEnabled {
		var oidcMissing []string
		for name, value := range map[string]string{
			"OIDC_ISSUER_URL":   c.OIDCIssuerURL,
			"OIDC_CLIENT_ID":    c.OIDCClientID,
			"OIDC_REDIRECT_URL": c.OIDCRedirectURL,
		} {
			if strings.TrimSpace(value) == "" {
				oidcMissing = append(oidcMissing, name)
			}
		}
		if len(oidcMissing) > 0 {
			return fmt.Errorf("config: OIDC_ENABLED=true 时缺失环境变量: %s", strings.Join(oidcMissing, ", "))
		}
		if c.Env == "production" {
			if strings.TrimSpace(c.OIDCClientSecret) == "" {
				return fmt.Errorf("config: production 要求 OIDC_CLIENT_SECRET 非空")
			}
			if !isHTTPSURL(c.OIDCIssuerURL) || !isHTTPSURL(c.OIDCRedirectURL) {
				return fmt.Errorf("config: production 要求 OIDC issuer 和 redirect 使用 HTTPS")
			}
			if weakSecret(c.OIDCClientSecret) {
				return fmt.Errorf("config: production 拒绝 OIDC 弱默认 client secret")
			}
		}
	}
	if strings.TrimSpace(c.SessionCookieName) == "" {
		return fmt.Errorf("config: SESSION_COOKIE_NAME 不能为空")
	}
	if c.SessionTTL <= 0 {
		return fmt.Errorf("config: SESSION_TTL 必须大于 0")
	}
	if c.ObservabilityDBInterval < 5*time.Second {
		return fmt.Errorf("config: OBSERVABILITY_DB_INTERVAL 不得小于 5s")
	}
	if c.OTelSampleRate < 0 || c.OTelSampleRate > 1 {
		return fmt.Errorf("config: OTEL_TRACE_SAMPLE_RATE 必须在 0..1")
	}
	if c.OTelEnabled && strings.TrimSpace(c.OTLPEndpoint) == "" {
		return fmt.Errorf("config: OTEL_ENABLED=true 时缺失环境变量: OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if !strings.HasPrefix(c.AuthPostLoginRedirect, "/") || strings.HasPrefix(c.AuthPostLoginRedirect, "//") {
		return fmt.Errorf("config: AUTH_POST_LOGIN_REDIRECT 必须是站内绝对路径")
	}
	return nil
}

func validateServiceURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("必须是绝对 HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("禁止 userinfo、query 或 fragment")
	}
	return nil
}

func validateTrustedOrigin(raw string, requireHTTPS bool) error {
	if strings.Contains(raw, "*") {
		return fmt.Errorf("禁止 wildcard")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("必须是绝对 HTTP(S) origin")
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return fmt.Errorf("production 必须使用 HTTPS origin")
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("禁止 userinfo、path、query 或 fragment")
	}
	return nil
}

func isHTTPSURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != ""
}

func weakSecret(raw string) bool {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case "changeme", "change-me", "password", "secret", "minioadmin",
		"minioadmin_dev", "wiki_dev_password", "ci-placeholder":
		return true
	default:
		return len(value) < 12
	}
}
