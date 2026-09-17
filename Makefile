# my_nav — 常用命令（Makefile 是唯一入口，避免散落的脚本）
SHELL := /bin/bash
WEB   := web
BIN   := bin/nav

# 开发期工具链：固定版本，装在仓库内 .tools/（不入库）
SQLC_VERSION := 1.31.1
SQLC         := .tools/sqlc

.PHONY: help
help: ## 显示所有可用目标
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'

.PHONY: tools
tools: $(SQLC) ## 下载开发期工具（sqlc）
$(SQLC):
	@mkdir -p .tools
	curl -sSL -o .tools/sqlc.tar.gz \
	  https://github.com/sqlc-dev/sqlc/releases/download/v$(SQLC_VERSION)/sqlc_$(SQLC_VERSION)_linux_amd64.tar.gz
	tar xzf .tools/sqlc.tar.gz -C .tools sqlc
	rm -f .tools/sqlc.tar.gz
	@$(SQLC) version

.PHONY: generate
generate: $(SQLC) ## 由 SQL 重新生成 internal/db/dbgen（生成物入库，CI 不需要 sqlc）
	$(SQLC) generate
	@echo "提示：生成物已入库，构建镜像时无需安装 sqlc。"

.PHONY: setup
setup: web-install ## 安装前后端依赖
	go mod download

.PHONY: web-install
web-install: ## 安装前端依赖
	npm --prefix $(WEB) install

.PHONY: dev
dev: ## 同时起 Vite(5180) 与 Go API(8080)；Ctrl-C 一起退出
	@trap 'kill 0' EXIT; \
	( cd $(WEB) && npm run dev ) & \
	( go run ./cmd/nav ) & \
	wait

.PHONY: web-dev
web-dev: ## 只起 Vite
	npm --prefix $(WEB) run dev

.PHONY: web-build
web-build: ## 构建前端 → internal/web/dist（会被 go:embed 打包）
	npm --prefix $(WEB) run build
	@# vite 的 emptyOutDir 会把 dist 清空，占位文件必须补回来：
	@# go:embed 不接受空目录，fresh clone 直接构建会失败；顺便也让工作区不脏。
	@mkdir -p internal/web/dist && touch internal/web/dist/.gitkeep

.PHONY: build
build: web-build ## 构建单二进制（前端已内嵌）
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o $(BIN) ./cmd/nav

.PHONY: build-go
build-go: ## 只编译 Go（沿用上一次前端产物 / placeholder）
	CGO_ENABLED=0 go build -trimpath -o $(BIN) ./cmd/nav

.PHONY: run
run: ## 本地运行（数据目录 ./data）
	NAV_ADDR=:8080 NAV_DATA_DIR=./data ./$(BIN)

.PHONY: e2e
e2e: ## 真浏览器验收（构建 + headless Chromium 跑完整交互，含拖拽）
	./e2e/run.sh

.PHONY: check
check: ## 前端类型检查 + Go vet
	npm --prefix $(WEB) run check
	go vet ./...

.PHONY: test
test: ## 跑 Go 测试
	go test ./... -count=1

.PHONY: fmt
fmt: ## 格式化
	gofmt -l -w cmd internal

.PHONY: clean
clean: ## 清理构建产物（保留 .gitkeep）
	rm -rf $(BIN) web/node_modules
	find internal/web/dist -mindepth 1 ! -name .gitkeep -delete
