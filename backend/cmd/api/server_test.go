package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anby/wiki/backend/internal/platform/observability"
	"github.com/go-chi/chi/v5"
)

// TestHealthz 冒烟测试：healthz handler 恒 200 并返回 service/version。
func TestHealthz(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	healthzHandler("wiki-api", "test")(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应非合法 JSON: %v", err)
	}
	if body["service"] != "wiki-api" || body["version"] != "test" {
		t.Errorf("响应内容不符合预期: %v", body)
	}
}

func TestRecovererRecordsPanicWithoutRawPath(t *testing.T) {
	metrics := observability.NewMetrics("wiki-api")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	router := chi.NewRouter()
	router.Use(RequestID)
	router.Use(metrics.HTTPMiddleware("wiki-api"))
	router.Use(Recoverer(logger, "wiki-api", metrics))
	router.Get("/panic/{id}", func(http.ResponseWriter, *http.Request) {
		panic("test panic")
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic/private-id", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("panic status=%d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	metrics.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	body := recorder.Body.String()
	if !strings.Contains(body, `wiki_http_panics_total{route="/panic/{id}",service="wiki-api"} 1`) {
		t.Fatalf("缺少 panic route-pattern 指标:\n%s", body)
	}
	if strings.Contains(body, "private-id") {
		t.Fatalf("panic 指标泄露原始路径:\n%s", body)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	metrics := observability.NewMetrics("wiki-api")
	router := NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), Deps{
		Service: "wiki-api",
		Version: "test",
		Metrics: metrics,
	}, nil, nil, nil, nil, nil)

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("healthz status=%d", recorder.Code)
	}
	recorder = httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics status=%d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `wiki_http_requests_total{method="GET",route="/healthz",service="wiki-api",status_class="2xx"} 1`) {
		t.Fatalf("缺少 route-pattern HTTP 指标:\n%s", body)
	}
}
