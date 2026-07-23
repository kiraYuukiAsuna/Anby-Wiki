package observability

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

func TestHTTPMiddlewareUsesRoutePattern(t *testing.T) {
	metrics := NewMetrics("test-api")
	router := chi.NewRouter()
	router.Use(metrics.HTTPMiddleware("test-api"))
	router.Get("/items/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	request := httptest.NewRequest(http.MethodGet, "/items/sensitive-123", nil)
	router.ServeHTTP(httptest.NewRecorder(), request)

	body := scrape(t, metrics)
	if !strings.Contains(body, `wiki_http_requests_total{method="GET",route="/items/{id}",service="test-api",status_class="2xx"} 1`) {
		t.Fatalf("指标未使用 chi route pattern:\n%s", body)
	}
	if strings.Contains(body, "sensitive-123") {
		t.Fatalf("指标泄露原始路径:\n%s", body)
	}
}

func TestMetricsRegistryIsIndependent(t *testing.T) {
	first := NewMetrics("first")
	second := NewMetrics("second")
	first.ObservePublish(time.Millisecond, nil)
	first.ObservePublish(time.Millisecond, errors.New("failed"))

	firstBody := scrape(t, first)
	secondBody := scrape(t, second)
	if !strings.Contains(firstBody, `result="success",service="first"`) ||
		!strings.Contains(firstBody, `result="failure",service="first"`) {
		t.Fatalf("第一个 registry 缺少发布结果:\n%s", firstBody)
	}
	if strings.Contains(secondBody, `service="first"`) {
		t.Fatalf("独立 registry 发生串扰:\n%s", secondBody)
	}
}

func TestInitTracingDisabledAndValidation(t *testing.T) {
	shutdown, err := InitTracing(context.Background(), TracingConfig{})
	if err != nil {
		t.Fatalf("禁用 tracing 不应失败: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("禁用 tracing shutdown 不应失败: %v", err)
	}
	if _, err := InitTracing(context.Background(), TracingConfig{Enabled: true}); err == nil {
		t.Fatal("启用 tracing 但缺少 endpoint 应失败")
	}
	if _, err := InitTracing(context.Background(), TracingConfig{
		Enabled: true, Endpoint: "http://localhost:4317", SampleRatio: 2,
	}); err == nil {
		t.Fatal("非法采样率应失败")
	}
}

func TestNormalizeOTLPEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		insecure bool
		want     string
	}{
		{name: "insecure bare endpoint", endpoint: "127.0.0.1:4317", insecure: true, want: "http://127.0.0.1:4317"},
		{name: "secure bare endpoint", endpoint: "collector:4317", want: "https://collector:4317"},
		{name: "explicit HTTP URL", endpoint: "http://collector:4317", want: "http://collector:4317"},
		{name: "explicit HTTPS URL", endpoint: "https://collector:4317", insecure: true, want: "https://collector:4317"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := normalizeOTLPEndpoint(test.endpoint, test.insecure); got != test.want {
				t.Fatalf("normalizeOTLPEndpoint() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestTraceResourceMergesWithoutSchemaConflict(t *testing.T) {
	res, err := traceResource(TracingConfig{
		Service:     "wiki-api",
		Version:     "test",
		Environment: "integration",
	})
	if err != nil {
		t.Fatalf("trace resource 不应产生 schema URL 冲突: %v", err)
	}
	if res == nil {
		t.Fatal("trace resource 不应为空")
	}
}

func scrape(t *testing.T, metrics *Metrics) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	response := recorder.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("读取 metrics 失败: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("metrics status=%d body=%s", response.StatusCode, body)
	}
	return string(body)
}
