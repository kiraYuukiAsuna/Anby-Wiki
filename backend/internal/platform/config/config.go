// Package config 从环境变量加载并校验应用配置。
package config

import (
	"fmt"
	"net"
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
	// SearchBackend selects the SearchAdapter implementation. The early stage
	// ships only the PostgreSQL FTS adapter; a dedicated engine can be added
	// back behind the same interface when capacity requires it (ADR-0006).
	SearchBackend string `env:"SEARCH_BACKEND" envDefault:"postgres"`
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
	// AuthDevLoginEnabled exposes POST /api/v1/auth/dev-login, which mints a
	// session from a shared bootstrap token instead of a verified identity.
	// This is an explicit early-stage placeholder with no identity
	// verification and must be replaced before public exposure.
	AuthDevLoginEnabled bool `env:"AUTH_DEV_LOGIN_ENABLED" envDefault:"true"`
	// AuthDevLoginToken is secret material and must never be logged. It is
	// mandatory whenever dev login is enabled outside development.
	AuthDevLoginToken string `env:"AUTH_DEV_LOGIN_TOKEN"`
	// RateLimitEnabled turns on the Redis-backed fixed-window limiter. The
	// application is now the only rate-limiting layer, because no reverse
	// proxy sits in front of it.
	RateLimitEnabled bool `env:"RATE_LIMIT_ENABLED" envDefault:"true"`
	// RateLimitGeneralPerMinute caps ordinary API requests per client.
	RateLimitGeneralPerMinute int `env:"RATE_LIMIT_GENERAL_PER_MINUTE" envDefault:"1200"`
	// RateLimitAuthPerMinute caps auth endpoints to slow credential stuffing.
	RateLimitAuthPerMinute int `env:"RATE_LIMIT_AUTH_PER_MINUTE" envDefault:"10"`
	// RateLimitUploadPerMinute caps upload endpoints.
	RateLimitUploadPerMinute int `env:"RATE_LIMIT_UPLOAD_PER_MINUTE" envDefault:"3"`
	// TrustedProxyIPs lists proxy addresses whose X-Forwarded-For may be
	// trusted for limiter client identity. Empty means use the socket peer.
	TrustedProxyIPs []string `env:"TRUSTED_PROXY_IPS" envSeparator:","`
	// AuthDevHeaderEnabled permits X-Actor-ID only in development/test.
	AuthDevHeaderEnabled bool          `env:"AUTH_DEV_HEADER_ENABLED" envDefault:"false"`
	SessionCookieName    string        `env:"SESSION_COOKIE_NAME" envDefault:"anby_session"`
	SessionCookieSecure  bool          `env:"SESSION_COOKIE_SECURE" envDefault:"true"`
	SessionTTL           time.Duration `env:"SESSION_TTL" envDefault:"24h"`
	// TrustedOrigins 是允许携带 session cookie 发起写请求的精确 HTTP(S) origin。
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
	default:
		return fmt.Errorf("config: 不支持的 SEARCH_BACKEND: %s", c.SearchBackend)
	}
	if c.RateLimitEnabled {
		for name, value := range map[string]int{
			"RATE_LIMIT_GENERAL_PER_MINUTE": c.RateLimitGeneralPerMinute,
			"RATE_LIMIT_AUTH_PER_MINUTE":    c.RateLimitAuthPerMinute,
			"RATE_LIMIT_UPLOAD_PER_MINUTE":  c.RateLimitUploadPerMinute,
		} {
			if value <= 0 {
				return fmt.Errorf("config: RATE_LIMIT_ENABLED=true 时 %s 必须大于 0", name)
			}
		}
	}
	if c.AuthDevLoginEnabled && c.Env != "development" {
		if strings.TrimSpace(c.AuthDevLoginToken) == "" {
			return fmt.Errorf("config: AUTH_DEV_LOGIN_ENABLED=true 且非 development 时要求 AUTH_DEV_LOGIN_TOKEN 非空")
		}
		if weakSecret(c.AuthDevLoginToken) {
			return fmt.Errorf("config: 拒绝弱 AUTH_DEV_LOGIN_TOKEN")
		}
	}
	for _, raw := range c.TrustedProxyIPs {
		if net.ParseIP(strings.TrimSpace(raw)) == nil {
			return fmt.Errorf("config: TRUSTED_PROXY_IPS 包含非法 IP %q", raw)
		}
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
		if c.AuthDevHeaderEnabled {
			return fmt.Errorf("config: production 严禁 AUTH_DEV_HEADER_ENABLED=true")
		}
		if len(c.TrustedOrigins) == 0 {
			return fmt.Errorf("config: production 要求 TRUSTED_ORIGINS")
		}
		if weakSecret(c.S3AccessKey) || weakSecret(c.S3SecretKey) {
			return fmt.Errorf("config: production 拒绝 S3 弱默认 Secret")
		}
	}
	for _, origin := range c.TrustedOrigins {
		if err := validateTrustedOrigin(origin); err != nil {
			return fmt.Errorf("config: TRUSTED_ORIGINS 包含非法 origin %q: %w", origin, err)
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
	return nil
}

func validateTrustedOrigin(raw string) error {
	if strings.Contains(raw, "*") {
		return fmt.Errorf("禁止 wildcard")
	}
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" ||
		(parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("必须是绝对 HTTP(S) origin")
	}
	if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("禁止 userinfo、path、query 或 fragment")
	}
	return nil
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
