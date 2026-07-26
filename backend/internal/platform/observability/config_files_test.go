package observability

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

// productionCompose is the only compose manifest in the repository. The local
// development stack no longer uses Docker, and the Prometheus/OTel sidecars
// were removed with it: metrics are exposed in-process on /metrics and OTLP is
// opt-in through OTEL_EXPORTER_OTLP_ENDPOINT.
func productionComposePath() string {
	return filepath.Join("..", "..", "..", "..", "infra", "deploy", "compose.production.yml")
}

func TestProductionComposeIsValidYAML(t *testing.T) {
	document := readYAMLMap(t, productionComposePath())
	if _, ok := document["services"]; !ok {
		t.Error("缺少顶层键 \"services\"")
	}
}

func TestProductionComposeExposesObservabilityContract(t *testing.T) {
	compose := readYAMLMap(t, productionComposePath())
	services := childMap(t, compose, "services")

	// The data tier and the application must all be declared.
	for _, service := range []string{"postgres", "redis", "minio", "api", "worker", "web"} {
		if _, ok := services[service]; !ok {
			t.Errorf("compose 缺少 %s service", service)
		}
	}
	// No reverse proxy: web is the only externally published service.
	if _, ok := services["nginx"]; ok {
		t.Error("compose 不应再包含反向代理 service")
	}
	// Search runs on PostgreSQL only.
	if _, ok := services["meilisearch"]; ok {
		t.Error("compose 不应再包含 meilisearch service")
	}

	worker := childMap(t, services, "worker")
	environment, ok := worker["environment"].(map[string]any)
	if !ok {
		t.Fatal("worker environment 不是 YAML map")
	}
	// Worker metrics must stay reachable for scraping.
	if addr, ok := environment["WORKER_METRICS_ADDR"]; !ok || addr == "" {
		t.Error("worker 缺少 WORKER_METRICS_ADDR")
	}
}

func readYAMLMap(t *testing.T, path string) map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 %s 失败: %v", path, err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(content, &document); err != nil {
		t.Fatalf("解析 %s 失败: %v", path, err)
	}
	return document
}

func childMap(t *testing.T, parent map[string]any, key string) map[string]any {
	t.Helper()
	child, ok := parent[key].(map[string]any)
	if !ok {
		t.Fatalf("%q 不是 YAML map", key)
	}
	return child
}
