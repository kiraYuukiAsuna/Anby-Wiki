package config

import (
	"strings"
	"testing"
	"time"
)

// setRequired 写入全部必填环境变量。
func setRequired(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://localhost:5432/wiki")
	t.Setenv("REDIS_URL", "redis://localhost:6379/0")
	t.Setenv("S3_ENDPOINT", "http://localhost:9000")
	t.Setenv("S3_BUCKET", "wiki")
	t.Setenv("S3_ACCESS_KEY", "ak")
	t.Setenv("S3_SECRET_KEY", "sk")
}

func setProductionRequired(t *testing.T) {
	t.Helper()
	setRequired(t)
	t.Setenv("ENV", "production")
	t.Setenv("S3_ACCESS_KEY", "production-access")
	t.Setenv("S3_SECRET_KEY", "production-secret-value")
	t.Setenv("OIDC_ENABLED", "true")
	t.Setenv("OIDC_ISSUER_URL", "https://id.example.com")
	t.Setenv("OIDC_CLIENT_ID", "wiki")
	t.Setenv("OIDC_CLIENT_SECRET", "production-oidc-secret")
	t.Setenv("OIDC_REDIRECT_URL", "https://wiki.example.com/api/v1/auth/callback")
	t.Setenv("TRUSTED_ORIGINS", "https://wiki.example.com")
	t.Setenv("SEARCH_BACKEND", "meilisearch")
	t.Setenv("MEILI_URL", "https://search.example.com")
	t.Setenv("MEILI_API_KEY", "production-meili-secret")
}

func TestLoad_Valid(t *testing.T) {
	setRequired(t)
	t.Setenv("PORT", "9090")
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("ENV", "staging")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() 返回错误: %v", err)
	}
	if cfg.Port != 9090 {
		t.Errorf("Port = %d, 期望 9090", cfg.Port)
	}
	if cfg.LogLevel != "debug" || cfg.Env != "staging" {
		t.Errorf("LogLevel/Env 解析错误: %+v", cfg)
	}
	if cfg.DatabaseURL == "" || cfg.RedisURL == "" || cfg.S3Bucket == "" {
		t.Errorf("必填字段未正确解析: %+v", cfg)
	}
}

func TestLoad_Defaults(t *testing.T) {
	setRequired(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() 返回错误: %v", err)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port 默认值 = %d, 期望 8080", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel 默认值 = %q, 期望 info", cfg.LogLevel)
	}
	if cfg.Env != "development" {
		t.Errorf("Env 默认值 = %q, 期望 development", cfg.Env)
	}
	if cfg.WorkerMetricsAddr != ":9091" || cfg.ObservabilityDBInterval != 30*time.Second {
		t.Errorf("observability 默认值错误: addr=%q interval=%s", cfg.WorkerMetricsAddr, cfg.ObservabilityDBInterval)
	}
	if cfg.OTelEnabled || cfg.OTelSampleRate != 1 {
		t.Errorf("OTel 默认值错误: enabled=%v sample=%v", cfg.OTelEnabled, cfg.OTelSampleRate)
	}
	if cfg.SearchBackend != "postgres" || cfg.MeiliIndex != "anby_pages" || cfg.MeiliTimeout != 15*time.Second {
		t.Errorf("搜索默认值错误: backend=%q index=%q timeout=%s",
			cfg.SearchBackend, cfg.MeiliIndex, cfg.MeiliTimeout)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	// 显式置空，避免受外部环境变量影响。
	for _, name := range []string{
		"DATABASE_URL", "REDIS_URL", "S3_ENDPOINT",
		"S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY",
	} {
		t.Setenv(name, "")
	}

	_, err := Load()
	if err == nil {
		t.Fatal("Load() 缺失必填项时应返回错误")
	}
	// 聚合错误应包含所有缺失字段名。
	for _, name := range []string{
		"DATABASE_URL", "REDIS_URL", "S3_ENDPOINT",
		"S3_BUCKET", "S3_ACCESS_KEY", "S3_SECRET_KEY",
	} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("错误信息缺少字段名 %s: %v", name, err)
		}
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	setRequired(t)
	t.Setenv("PORT", "not-a-number")

	if _, err := Load(); err == nil {
		t.Fatal("Load() 非法 PORT 时应返回错误")
	}
}

func TestLoad_AIImportRequiresProviderConfiguration(t *testing.T) {
	setRequired(t)
	t.Setenv("AI_IMPORT_ENABLED", "true")
	for _, name := range []string{"AI_BASE_URL", "AI_API_KEY", "AI_MODEL"} {
		t.Setenv(name, "")
	}
	_, err := Load()
	if err == nil {
		t.Fatal("启用 AI 导入但缺少供应商配置时应返回错误")
	}
	for _, name := range []string{"AI_BASE_URL", "AI_API_KEY", "AI_MODEL"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("错误信息缺少字段名 %s: %v", name, err)
		}
	}
}

func TestLoad_ProductionRejectsDevelopmentActorHeader(t *testing.T) {
	setProductionRequired(t)
	t.Setenv("AUTH_DEV_HEADER_ENABLED", "true")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "严禁 AUTH_DEV_HEADER_ENABLED") {
		t.Fatalf("production 应拒绝开发身份头，err=%v", err)
	}
}

func TestLoad_SearchBackendValidation(t *testing.T) {
	t.Run("unsupported", func(t *testing.T) {
		setRequired(t)
		t.Setenv("SEARCH_BACKEND", "elastic")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SEARCH_BACKEND") {
			t.Fatalf("不支持的搜索后端应失败，err=%v", err)
		}
	})
	t.Run("invalid Meili URL", func(t *testing.T) {
		setRequired(t)
		t.Setenv("SEARCH_BACKEND", "meilisearch")
		t.Setenv("MEILI_URL", "http://user:secret@localhost:7700")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MEILI_URL") {
			t.Fatalf("带凭据的 Meili URL 应失败，err=%v", err)
		}
	})
	t.Run("production requires Meili", func(t *testing.T) {
		setProductionRequired(t)
		t.Setenv("SEARCH_BACKEND", "postgres")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SEARCH_BACKEND=meilisearch") {
			t.Fatalf("production 应强制 Meili，err=%v", err)
		}
	})
	t.Run("production requires API key", func(t *testing.T) {
		setProductionRequired(t)
		t.Setenv("MEILI_API_KEY", "")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "MEILI_API_KEY") {
			t.Fatalf("production 应要求 Meili API key，err=%v", err)
		}
	})
}

func TestLoad_ProductionRequiresOIDCAndSecureCookie(t *testing.T) {
	setProductionRequired(t)
	t.Setenv("OIDC_ENABLED", "false")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OIDC_ENABLED=true") {
		t.Fatalf("production 应要求 OIDC，err=%v", err)
	}

	t.Setenv("OIDC_ENABLED", "true")
	t.Setenv("OIDC_ISSUER_URL", "https://id.example.com")
	t.Setenv("OIDC_CLIENT_ID", "wiki")
	t.Setenv("OIDC_REDIRECT_URL", "https://wiki.example.com/api/v1/auth/callback")
	t.Setenv("SESSION_COOKIE_SECURE", "false")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_COOKIE_SECURE=true") {
		t.Fatalf("production 应要求 Secure cookie，err=%v", err)
	}
}

func TestLoad_ProductionRequiresStrictTrustedOrigins(t *testing.T) {
	tests := []struct {
		name    string
		origins string
		want    string
	}{
		{name: "missing", origins: "", want: "TRUSTED_ORIGINS"},
		{name: "http", origins: "http://wiki.example.com", want: "HTTPS"},
		{name: "path", origins: "https://wiki.example.com/app", want: "path"},
		{name: "wildcard", origins: "https://*.example.com", want: "wildcard"},
		{name: "query", origins: "https://wiki.example.com?tenant=1", want: "query"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setProductionRequired(t)
			t.Setenv("TRUSTED_ORIGINS", tt.origins)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("TRUSTED_ORIGINS=%q 应失败并包含 %q，err=%v", tt.origins, tt.want, err)
			}
		})
	}
}

func TestLoad_ProductionRejectsWeakSecretsAndInsecureOIDC(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "weak s3", key: "S3_SECRET_KEY", value: "minioadmin_dev", want: "S3 弱默认"},
		{name: "empty client secret", key: "OIDC_CLIENT_SECRET", value: "", want: "OIDC_CLIENT_SECRET"},
		{name: "weak client secret", key: "OIDC_CLIENT_SECRET", value: "changeme", want: "OIDC 弱默认"},
		{name: "http issuer", key: "OIDC_ISSUER_URL", value: "http://id.example.com", want: "HTTPS"},
		{name: "http redirect", key: "OIDC_REDIRECT_URL", value: "http://wiki.example.com/callback", want: "HTTPS"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setProductionRequired(t)
			t.Setenv(tt.key, tt.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("%s=%q 应失败并包含 %q，err=%v", tt.key, tt.value, tt.want, err)
			}
		})
	}
}

func TestLoad_OIDCRequiresProtocolConfiguration(t *testing.T) {
	setRequired(t)
	t.Setenv("OIDC_ENABLED", "true")
	t.Setenv("OIDC_ISSUER_URL", "")
	t.Setenv("OIDC_CLIENT_ID", "")
	t.Setenv("OIDC_REDIRECT_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("启用 OIDC 但缺少协议配置时应返回错误")
	}
	for _, name := range []string{"OIDC_ISSUER_URL", "OIDC_CLIENT_ID", "OIDC_REDIRECT_URL"} {
		if !strings.Contains(err.Error(), name) {
			t.Errorf("错误信息缺少字段名 %s: %v", name, err)
		}
	}
}

func TestLoad_ObservabilityValidation(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
		want  string
	}{
		{name: "interval too short", key: "OBSERVABILITY_DB_INTERVAL", value: "1s", want: "不得小于 5s"},
		{name: "sample negative", key: "OTEL_TRACE_SAMPLE_RATE", value: "-0.1", want: "必须在 0..1"},
		{name: "sample above one", key: "OTEL_TRACE_SAMPLE_RATE", value: "1.1", want: "必须在 0..1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setRequired(t)
			t.Setenv(tt.key, tt.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("%s=%q 应失败并包含 %q，err=%v", tt.key, tt.value, tt.want, err)
			}
		})
	}

	t.Run("enabled requires endpoint", func(t *testing.T) {
		setRequired(t)
		t.Setenv("OTEL_ENABLED", "true")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
		if _, err := Load(); err == nil || !strings.Contains(err.Error(), "OTEL_EXPORTER_OTLP_ENDPOINT") {
			t.Fatalf("启用 OTel 缺少 endpoint 应失败，err=%v", err)
		}
	})

	t.Run("enabled valid", func(t *testing.T) {
		setRequired(t)
		t.Setenv("OTEL_ENABLED", "true")
		t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4317")
		t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
		t.Setenv("OTEL_TRACE_SAMPLE_RATE", "0.25")
		cfg, err := Load()
		if err != nil {
			t.Fatalf("有效 OTel 配置不应失败: %v", err)
		}
		if !cfg.OTLPInsecure || cfg.OTelSampleRate != 0.25 {
			t.Fatalf("OTel 配置解析错误: %+v", cfg)
		}
	})
}
