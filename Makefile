.DEFAULT_GOAL := help

# OpenAPI Generator 需要 Java；Homebrew openjdk 未链接进 PATH 时从 keg 补齐
ifneq ($(wildcard /opt/homebrew/opt/openjdk/bin/java),)
export PATH := /opt/homebrew/opt/openjdk/bin:$(PATH)
endif

COMPOSE := docker compose -f infra/local/docker-compose.yml
WEB_DIR := apps/web
BACKEND_DIR := backend
GOVULNCHECK_VERSION := v1.1.4
GITLEAKS_VERSION := v8.28.0

.PHONY: help bootstrap infra-up infra-down infra-ps infra-observability-up migrate-up migrate-down \
        dev dev-api dev-worker dev-web lint lint-go lint-web \
        test test-go test-web test-e2e build build-go build-web \
        gen-client contract-schema-check observability-config-check gen-check security security-go \
        security-web security-secrets deploy-config-check perf-db perf-smoke perf-full perf-meili-full ci

help: ## 显示全部可用命令
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

bootstrap: ## 安装前后端依赖
	cd $(BACKEND_DIR) && go mod download
	cd $(WEB_DIR) && npm ci

# ---- 本地基础设施 ----

infra-up: ## 启动 PostgreSQL / Redis / Meilisearch / MinIO
	$(COMPOSE) up -d --wait

infra-down: ## 停止本地基础设施
	$(COMPOSE) down

infra-ps: ## 查看基础设施状态
	$(COMPOSE) ps

infra-observability-up: ## 启动本地 Prometheus / OTel Collector
	$(COMPOSE) --profile observability up -d prometheus otel-collector

pg-start: ## 启动本地免 Docker PostgreSQL（Homebrew 版，端口 55432）
	sh scripts/dev-pg.sh start

pg-stop: ## 停止本地 PostgreSQL
	sh scripts/dev-pg.sh stop

pg-reset: ## 重置本地 PostgreSQL 数据并重新迁移
	sh scripts/dev-pg.sh reset

# ---- 数据库迁移 ----

migrate-up: ## 执行全部迁移
	$(COMPOSE) exec -T postgres sh -c 'true' # 确保依赖已启动
	cd $(BACKEND_DIR) && go run ./cmd/migrate up

migrate-down: ## 回滚一步迁移
	cd $(BACKEND_DIR) && go run ./cmd/migrate down 1

# ---- 开发 ----

dev: ## 并行启动 API / Worker / Web
	$(MAKE) -j3 dev-api dev-worker dev-web

dev-api:
	cd $(BACKEND_DIR) && go run ./cmd/api

dev-worker:
	cd $(BACKEND_DIR) && go run ./cmd/worker

dev-web:
	cd $(WEB_DIR) && npm run dev

# ---- 质量门禁 ----

lint: lint-go lint-web ## 全部静态检查

lint-go:
	cd $(BACKEND_DIR) && gofmt -l . | grep . && exit 1 || true
	cd $(BACKEND_DIR) && go vet ./...

lint-web:
	cd $(WEB_DIR) && npm run lint

test: test-go test-web ## 全部测试

# 集成测试共享同一个 TEST_DATABASE_URL 且每个用例 Reset 全库，
# 包间并行会互相 TRUNCATE，因此集成测试必须 -p 1 串行（单元测试不受影响）。
test-go:
	cd $(BACKEND_DIR) && go test ./...

test-go-integration: ## 带 TEST_DATABASE_URL 的集成测试（串行）
	cd $(BACKEND_DIR) && go test ./... -count=1 -p 1

test-web:
	cd $(WEB_DIR) && npm run test

test-e2e: ## Playwright 浏览器生命周期（需 PostgreSQL 已迁移及 Chromium）
	cd $(WEB_DIR) && npm run test:e2e

build: build-go build-web ## 全部构建

build-go:
	cd $(BACKEND_DIR) && go build ./...

build-web:
	cd $(WEB_DIR) && npm run build

# ---- 安全与供应链 ----

security: security-go security-web security-secrets ## 依赖完整性、漏洞、生产依赖与密钥扫描

security-go:
	cd $(BACKEND_DIR) && go mod verify
	cd $(BACKEND_DIR) && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

security-web:
	cd $(WEB_DIR) && npm audit --omit=dev --audit-level=high

security-secrets:
	go run github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION) dir . --config .gitleaks.toml --no-banner --redact

# ---- 契约与客户端 ----

gen-client: ## 从 OpenAPI 生成 TypeScript 客户端
	cd $(WEB_DIR) && npm run gen:client

contract-schema-check: ## 校验 AST / ProposalOperation / Extraction 权威 Schema 与 Go 内嵌副本
	sh scripts/check-ast-schema-sync.sh
	sh scripts/check-operation-schema-sync.sh
	sh scripts/check-extraction-schema-sync.sh

observability-config-check: ## 静态校验观测配置；Docker 可用时追加官方校验
	sh scripts/check-observability-config.sh

deploy-config-check: ## 校验 OCI/Compose/迁移 gate 与发布顺序；Docker 可用时追加 Compose 校验
	sh scripts/check-deploy-config.sh

perf-db: ## 重建并迁移独立性能库 wiki_perf_m7t05
	sh scripts/perf-db.sh

perf-smoke: ## 在独立性能库运行小规模快速基准
	cd $(BACKEND_DIR) && PERF_DATABASE_CONFIRM=ANBY_WIKI_PERF_ONLY go run ./cmd/perf -profile smoke

perf-full: ## 在独立性能库运行 100k 页面完整基准
	cd $(BACKEND_DIR) && PERF_DATABASE_CONFIRM=ANBY_WIKI_PERF_ONLY go run ./cmd/perf -profile full -output /tmp/anby-wiki-m7-t05-full.json

perf-meili-full: ## 在独立性能库与 Meilisearch 运行 100k 完整基准
	cd $(BACKEND_DIR) && PERF_DATABASE_CONFIRM=ANBY_WIKI_PERF_ONLY PERF_SEARCH_BACKEND=meilisearch \
		MEILI_URL=$${MEILI_URL:-http://localhost:7700} MEILI_INDEX=$${MEILI_INDEX:-anby_pages_perf} \
		go run ./cmd/perf -profile full -output /tmp/anby-wiki-m7-t11-meili-full.json

gen-check: contract-schema-check ## CI：Schema 副本与生成物漂移检查
	cd $(WEB_DIR) && npm run gen:client
	git diff --exit-code contracts/generated/typescript

ci: lint test build gen-check observability-config-check deploy-config-check security ## 本地等价 CI
