#!/usr/bin/env bash
# 真浏览器验收：构建 → 起一个临时数据目录的后端 → headless Chromium 跑 smoke.mjs → 收尾。
# 用法：./e2e/run.sh [端口]
set -euo pipefail

cd "$(dirname "$0")/.."
PORT="${1:-18090}"
ROOT="$(pwd)"
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

SMOKE_DIR="$ROOT/.smoke"
rm -rf "$SMOKE_DIR" && mkdir -p "$SMOKE_DIR"
echo "==> 启动后端 :$PORT（数据目录 $SMOKE_DIR）"
NAV_ADDR="127.0.0.1:$PORT" NAV_DATA_DIR="$SMOKE_DIR" ./bin/nav > "$SMOKE_DIR/server.log" 2>&1 &
SRV=$!
trap 'kill $SRV 2>/dev/null || true' EXIT

for _ in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:$PORT/healthz" >/dev/null; then break; fi
  sleep 0.3
done

NAV_BASE="http://127.0.0.1:$PORT" node e2e/smoke.mjs
