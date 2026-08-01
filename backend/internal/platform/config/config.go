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
	// APIPort Go API HTTP 监听端口，默认 8080。
	APIPort int `env:"API_PORT" envDefault:"8080"`
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
	// SearchBackend selects PostgreSQL staging/fallback or the independent
	// Meilisearch query/index adapter.
	SearchBackend         string        `env:"SEARCH_BACKEND" envDefault:"postgres"`
	MeiliURL              string        `env:"MEILI_URL" envDefault:"http://localhost:7700"`
	MeiliAPIKey           string        `env:"MEILI_API_KEY"`
	MeiliIndex            string        `env:"MEILI_INDEX" envDefault:"anby_pages"`
	MeiliTimeout          time.Duration `env:"MEILI_TIMEOUT" envDefault:"15s"`
	MeiliTaskTimeout      time.Duration `env:"MEILI_TASK_TIMEOUT" envDefault:"10m"`
	SearchSemanticEnabled bool          `env:"SEARCH_SEMANTIC_ENABLED" envDefault:"false"`
	MeiliEmbedderName     string        `env:"MEILI_EMBEDDER_NAME" envDefault:"page-content"`
	MeiliEmbedderSource   string        `env:"MEILI_EMBEDDER_SOURCE" envDefault:"huggingFace"`
	MeiliEmbedderModel    string        `env:"MEILI_EMBEDDER_MODEL" envDefault:"sentence-transformers/paraphrase-multilingual-MiniLM-L12-v2"`
	// MeiliEmbedderAPIKey is required only for the openAi embedder source.
	// It is secret material and must never be logged.
	MeiliEmbedderAPIKey string `env:"MEILI_EMBEDDER_API_KEY"`
	// WorkerMetricsAddr 是 Worker 独立指标监听地址；空字符串可显式关闭。
	WorkerMetricsAddr string `env:"WORKER_METRICS_ADDR" envDefault:":9091"`
	// RevisionArchiveEnabled enables the worker's recurring hot-to-cold
	// migration for non-current immutable ContentSnapshots.
	RevisionArchiveEnabled   bool          `env:"REVISION_ARCHIVE_ENABLED" envDefault:"false"`
	RevisionArchiveRetention time.Duration `env:"REVISION_ARCHIVE_RETENTION" envDefault:"4320h"`
	RevisionArchiveInterval  time.Duration `env:"REVISION_ARCHIVE_INTERVAL" envDefault:"6h"`
	RevisionArchiveBatchSize int           `env:"REVISION_ARCHIVE_BATCH_SIZE" envDefault:"50"`
	// RevisionArchiveMaxBytes bounds a single cold object during both archive
	// and transparent history hydration.
	RevisionArchiveMaxBytes int64 `env:"REVISION_ARCHIVE_MAX_BYTES" envDefault:"67108864"`
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
	// AIProvider 是 Gateway 内的供应商键。内置 openai-compatible 与 deepseek Adapter。
	AIProvider string `env:"AI_PROVIDER" envDefault:"openai-compatible"`
	// AIBaseURL 是模型供应商 API 根地址（例如 https://host/v1）。
	AIBaseURL string `env:"AI_BASE_URL"`
	// AIAPIKey 只从进程环境注入；生产可由受保护的部署环境文件提供，禁止写入日志。
	AIAPIKey string `env:"AI_API_KEY"`
	// AIModel 是导入抽取使用的模型 ID。
	AIModel string `env:"AI_MODEL"`
	// AuthRegistrationEnabled controls public local-account registration.
	// Operators may disable it after provisioning the required accounts.
	AuthRegistrationEnabled bool `env:"AUTH_REGISTRATION_ENABLED" envDefault:"false"`
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
		if c.SearchSemanticEnabled {
			return fmt.Errorf("config: SEARCH_SEMANTIC_ENABLED=true 要求 SEARCH_BACKEND=meilisearch")
		}
	case "meilisearch":
		if strings.TrimSpace(c.MeiliIndex) == "" {
			return fmt.Errorf("config: SEARCH_BACKEND=meilisearch 时 MEILI_INDEX 不能为空")
		}
		if c.MeiliTimeout <= 0 {
			return fmt.Errorf("config: MEILI_TIMEOUT 必须大于 0")
		}
		if c.MeiliTaskTimeout <= 0 {
			return fmt.Errorf("config: MEILI_TASK_TIMEOUT 必须大于 0")
		}
		if err := validateServiceURL(c.MeiliURL); err != nil {
			return fmt.Errorf("config: MEILI_URL 非法: %w", err)
		}
		if c.SearchSemanticEnabled {
			if strings.TrimSpace(c.MeiliEmbedderName) == "" {
				return fmt.Errorf("config: 语义搜索启用时 MEILI_EMBEDDER_NAME 不能为空")
			}
			if strings.TrimSpace(c.MeiliEmbedderModel) == "" {
				return fmt.Errorf("config: 语义搜索启用时 MEILI_EMBEDDER_MODEL 不能为空")
			}
			switch c.MeiliEmbedderSource {
			case "huggingFace":
			case "openAi":
				if strings.TrimSpace(c.MeiliEmbedderAPIKey) == "" {
					return fmt.Errorf("config: MEILI_EMBEDDER_SOURCE=openAi 时 MEILI_EMBEDDER_API_KEY 不能为空")
				}
			default:
				return fmt.Errorf("config: 不支持的 MEILI_EMBEDDER_SOURCE: %s", c.MeiliEmbedderSource)
			}
		}
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
		if c.AIProvider != "openai-compatible" && c.AIProvider != "deepseek" {
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
		if !c.SearchSemanticEnabled {
			return fmt.Errorf("config: production 要求 SEARCH_SEMANTIC_ENABLED=true")
		}
		if c.AuthDevHeaderEnabled {
			return fmt.Errorf("config: production 严禁 AUTH_DEV_HEADER_ENABLED=true")
		}
		if weakSecret(c.S3AccessKey) || weakSecret(c.S3SecretKey) {
			return fmt.Errorf("config: production 拒绝 S3 弱默认 Secret")
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
	if c.RevisionArchiveRetention <= 0 {
		return fmt.Errorf("config: REVISION_ARCHIVE_RETENTION 必须大于 0")
	}
	if c.RevisionArchiveInterval < time.Minute {
		return fmt.Errorf("config: REVISION_ARCHIVE_INTERVAL 不得小于 1m")
	}
	if c.RevisionArchiveBatchSize < 1 || c.RevisionArchiveBatchSize > 500 {
		return fmt.Errorf("config: REVISION_ARCHIVE_BATCH_SIZE 必须在 1..500")
	}
	if c.RevisionArchiveMaxBytes <= 0 {
		return fmt.Errorf("config: REVISION_ARCHIVE_MAX_BYTES 必须大于 0")
	}
	if c.OTelSampleRate < 0 || c.OTelSampleRate > 1 {
		return fmt.Errorf("config: OTEL_TRACE_SAMPLE_RATE 必须在 0..1")
	}
	if c.OTelEnabled && strings.TrimSpace(c.OTLPEndpoint) == "" {
		return fmt.Errorf("config: OTEL_ENABLED=true 时缺失环境变量: OTEL_EXPORTER_OTLP_ENDPOINT")
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
