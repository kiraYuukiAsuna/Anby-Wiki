.DEFAULT_GOAL := help

WEB_DIR := apps/web
BACKEND_DIR := backend
SH := sh
GOVULNCHECK_VERSION := v1.1.4
GITLEAKS_VERSION := v8.28.0

# Homebrew 的 Java 通常不进入默认 PATH；保留该路径以兼容 macOS 本地生成客户端。
JAVA_HOMEBREW_BIN := /opt/homebrew/opt/openjdk/bin
ifneq ("$(wildcard $(JAVA_HOMEBREW_BIN)/java)","")
export PATH := $(JAVA_HOMEBREW_BIN):$(PATH)
endif

.PHONY: \
	help bootstrap \
	dev dev-api dev-worker dev-web \
	pg-start pg-stop pg-reset migrate-up migrate-down migrate-version migration-check \
	format-check typecheck lint lint-go lint-web shell-check \
	build build-go build-web check \
	gen-client contracts-check gen-check \
	deploy-check \
	security security-go security-web security-secrets \
	perf-db perf-smoke perf-full \
	ci \
	contract-schema-check deploy-config-check

## Makefile 是仓库命令的公共入口；scripts/ 保存实现脚本与需显式执行的运维操作。

help: ## 显示分组命令帮助
	@awk 'BEGIN {FS = ":.*## "; printf "\nUsage: make <target>\n"} \
		/^##@/ {printf "\n\033[1m%s\033[0m\n", substr($$0, 5); next} \
		/^[a-zA-Z0-9_-]+:.*## / {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}' \
		$(MAKEFILE_LIST)

##@ 初始化与开发

bootstrap: ## 安装 Go 与 Web 依赖
	cd $(BACKEND_DIR) && go mod download
	cd $(WEB_DIR) && npm ci

dev: ## 从根目录 .env 启动 API、Worker 与 Web
	$(SH) scripts/dev.sh

dev-api: ## 使用当前 shell 环境启动 API
	cd $(BACKEND_DIR) && go run ./cmd/api

dev-worker: ## 使用当前 shell 环境启动 Worker
	cd $(BACKEND_DIR) && go run ./cmd/worker

dev-web: ## 使用当前 shell 环境启动 Web
	cd $(WEB_DIR) && npm run dev

##@ 本地数据库

pg-start: ## 启动免 Docker 的本地 PostgreSQL
	$(SH) scripts/dev-pg.sh start

pg-stop: ## 停止本地 PostgreSQL
	$(SH) scripts/dev-pg.sh stop

pg-reset: ## 重建本地 PostgreSQL 数据库
	$(SH) scripts/dev-pg.sh reset

migrate-up: ## 执行全部向上迁移
	cd $(BACKEND_DIR) && go run ./cmd/migrate up

migrate-down: ## 回滚一步迁移
	cd $(BACKEND_DIR) && go run ./cmd/migrate down 1

migrate-version: ## 显示当前迁移版本
	cd $(BACKEND_DIR) && go run ./cmd/migrate version

migration-check: ## 校验迁移文件命名、配对与连续性
	$(SH) scripts/check-migrations.sh

##@ 质量检查

format-check: ## 检查 Go 文件是否经过 gofmt
	$(SH) scripts/check-go-format.sh

typecheck: ## 执行 Web TypeScript 类型检查
	cd $(WEB_DIR) && npm run typecheck

lint: lint-go lint-web ## 执行 Go 与 Web 静态检查

lint-go: format-check ## 执行 Go 格式与 vet 检查
	cd $(BACKEND_DIR) && go vet ./...

lint-web: typecheck ## 执行 Web 类型与 ESLint 检查
	cd $(WEB_DIR) && npm run lint

shell-check: ## 检查 Shell 脚本语法
	find scripts infra/deploy -type f -name '*.sh' -exec $(SH) -n {} \;

build: build-go build-web ## 构建 Go 与 Web

build-go: ## 构建全部 Go 命令与包
	cd $(BACKEND_DIR) && go build ./...

build-web: ## 构建 Next.js Web
	cd $(WEB_DIR) && npm run build

check: lint build contracts-check migration-check deploy-check ## 执行提交前静态与构建门禁

##@ 契约与生成物

gen-client: ## 从 OpenAPI 重新生成 TypeScript 客户端
	cd $(WEB_DIR) && npm run gen:client

contracts-check: ## 校验权威 Schema 与嵌入副本一致
	$(SH) scripts/check-contracts.sh

gen-check: contracts-check gen-client ## 校验生成客户端没有漂移
	git diff --exit-code -- contracts/generated/typescript

##@ 部署与运维检查

deploy-check: shell-check ## 静态校验生产部署与运维脚本
	$(SH) scripts/check-deploy-config.sh

##@ 安全

security: security-go security-web security-secrets ## 执行依赖与密钥扫描

security-go:
	cd $(BACKEND_DIR) && go mod verify
	cd $(BACKEND_DIR) && go run golang.org/x/vuln/cmd/govulncheck@$(GOVULNCHECK_VERSION) ./...

security-web:
	cd $(WEB_DIR) && npm audit --omit=dev --audit-level=high

security-secrets:
	go run github.com/zricethezav/gitleaks/v8@$(GITLEAKS_VERSION) dir . --config .gitleaks.toml --no-banner --redact

##@ 性能

perf-db: ## 重建独立性能基准数据库
	$(SH) scripts/perf-db.sh

perf-smoke: ## 执行性能冒烟
	cd $(BACKEND_DIR) && PERF_DATABASE_CONFIRM=ANBY_WIKI_PERF_ONLY go run ./cmd/perf -profile smoke

perf-full: ## 执行完整性能压测
	cd $(BACKEND_DIR) && PERF_DATABASE_CONFIRM=ANBY_WIKI_PERF_ONLY go run ./cmd/perf -profile full -output /tmp/anby-wiki-m7-t05-full.json

##@ 汇总

ci: check gen-check security ## 执行本地等价 CI

# 兼容旧入口；不在 help 中展示。
contract-schema-check: contracts-check
deploy-config-check: deploy-check
