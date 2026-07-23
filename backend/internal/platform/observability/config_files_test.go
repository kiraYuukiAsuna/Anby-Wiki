package observability

import (
	"os"
	"path/filepath"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestObservabilityConfigYAML(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..")
	tests := []struct {
		path     string
		required []string
	}{
		{
			path:     filepath.Join(root, "infra", "local", "docker-compose.yml"),
			required: []string{"services"},
		},
		{
			path:     filepath.Join(root, "infra", "local", "observability", "prometheus.yml"),
			required: []string{"scrape_configs", "rule_files"},
		},
		{
			path:     filepath.Join(root, "infra", "local", "observability", "alerts.yml"),
			required: []string{"groups"},
		},
		{
			path:     filepath.Join(root, "infra", "local", "observability", "otel-collector.yml"),
			required: []string{"receivers", "processors", "exporters", "service"},
		},
	}
	for _, test := range tests {
		t.Run(filepath.Base(test.path), func(t *testing.T) {
			content, err := os.ReadFile(test.path)
			if err != nil {
				t.Fatalf("读取配置失败: %v", err)
			}
			var document map[string]any
			if err := yaml.Unmarshal(content, &document); err != nil {
				t.Fatalf("YAML 非法: %v", err)
			}
			for _, key := range test.required {
				if _, ok := document[key]; !ok {
					t.Errorf("缺少顶层键 %q", key)
				}
			}
		})
	}
}

func TestObservabilityConfigStructure(t *testing.T) {
	root := filepath.Join("..", "..", "..", "..", "infra", "local")
	compose := readYAMLMap(t, filepath.Join(root, "docker-compose.yml"))
	services := childMap(t, compose, "services")
	for _, service := range []string{"prometheus", "otel-collector"} {
		if _, ok := services[service]; !ok {
			t.Errorf("compose 缺少 %s service", service)
		}
	}

	prometheus := readYAMLMap(t, filepath.Join(root, "observability", "prometheus.yml"))
	if scrapes, ok := prometheus["scrape_configs"].([]any); !ok || len(scrapes) != 2 {
		t.Errorf("Prometheus 应配置 API 与 Worker 两个 scrape job")
	}
	if rules, ok := prometheus["rule_files"].([]any); !ok || len(rules) == 0 {
		t.Errorf("Prometheus 缺少 rule_files")
	}

	alerts := readYAMLMap(t, filepath.Join(root, "observability", "alerts.yml"))
	if groups, ok := alerts["groups"].([]any); !ok || len(groups) == 0 {
		t.Errorf("alerts 缺少规则组")
	}

	collector := readYAMLMap(t, filepath.Join(root, "observability", "otel-collector.yml"))
	service := childMap(t, collector, "service")
	pipelines := childMap(t, service, "pipelines")
	if _, ok := pipelines["traces"]; !ok {
		t.Errorf("OTel Collector 缺少 traces pipeline")
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
