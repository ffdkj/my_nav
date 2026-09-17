#!/usr/bin/env bash
# 真浏览器验收：构建一次，然后**每个用例用独立的数据目录与端口**各起一个后端，
# 保证用例之间互不污染、也能单独跑。
#
# 用法：./e2e/run.sh [起始端口]
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$(pwd)"
BASE_PORT="${1:-18090}"
export PLAYWRIGHT_BROWSERS_PATH="${PLAYWRIGHT_BROWSERS_PATH:-$ROOT/e2e/browsers}"

if [ ! -d e2e/node_modules ]; then
  echo "==> 安装 e2e 依赖（playwright）"
  npm --prefix e2e install --no-audit --no-fund
fi
if [ ! -d "$PLAYWRIGHT_BROWSERS_PATH" ]; then
  echo "==> 下载 chromium 到 $PLAYWRIGHT_BROWSERS_PATH"
  npm --prefix e2e exec -- playwright install chromium
fi

echo "==> 构建前端 + 后端"
npm --prefix web run build >/dev/null
CGO_ENABLED=0 go build -o bin/nav ./cmd/nav

FAILED=0
run_case() { # 名称 端口 脚本
  local name="$1" port="$2" script="$3"
  local dir="$ROOT/.smoke/$name"
  rm -rf "$dir"; mkdir -p "$dir"
  echo "==> [$name] 启动后端 :$port（数据目录 .smoke/$name）"
  NAV_ADDR="127.0.0.1:$port" NAV_DATA_DIR="$dir" ./bin/nav > "$dir/server.log" 2>&1 &
  local srv=$!
  for _ in $(seq 1 40); do
    curl -sf "http://127.0.0.1:$port/healthz" >/dev/null && break
    sleep 0.25
  done
  NAV_BASE="http://127.0.0.1:$port" node "e2e/$script" || FAILED=1
  kill "$srv" 2>/dev/null || true
  wait "$srv" 2>/dev/null || true
}

run_case basic   "$BASE_PORT"       smoke.mjs
run_case folders "$((BASE_PORT+1))" folders.mjs

echo
if [ "$FAILED" -eq 0 ]; then echo "全部用例通过"; else echo "有用例失败"; fi
exit "$FAILED"
