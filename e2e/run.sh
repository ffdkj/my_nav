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

# run_case 名称 端口 脚本 [--addr 监听地址] [环境赋值...]
#
# 每个用例：独立数据目录 → 起后端 → 等健康检查 → 跑 Playwright → 收尾。
# --addr 0.0.0.0 用于 PWA 用例：它需要同时从回环（安全上下文）与局域网 IP
# （非安全上下文）访问，以验证真实部署下的降级路径。
run_case() {
  local name="$1" port="$2" script="$3"
  shift 3
  local addr="127.0.0.1"
  local envs=()
  while [ $# -gt 0 ]; do
    case "$1" in
      --addr) addr="$2"; shift 2 ;;
      *) envs+=("$1"); shift ;;
    esac
  done

  local dir="$ROOT/.smoke/$name"
  local label=""
  [ "${#envs[@]}" -gt 0 ] && label=" · ${envs[*]}"
  rm -rf "$dir"; mkdir -p "$dir"
  echo "==> [$name] 后端 $addr:$port（数据目录 .smoke/$name）$label"

  env NAV_ADDR="$addr:$port" NAV_DATA_DIR="$dir" "${envs[@]+"${envs[@]}"}" \
    ./bin/nav > "$dir/server.log" 2>&1 &
  local srv=$!
  for _ in $(seq 1 40); do
    curl -sf "http://127.0.0.1:$port/healthz" >/dev/null && break
    sleep 0.25
  done

  local lan_base=""
  if [ "$addr" = "0.0.0.0" ]; then
    local lan_ip
    lan_ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
    [ -n "$lan_ip" ] && lan_base="http://$lan_ip:$port"
  fi

  NAV_BASE="http://127.0.0.1:$port" \
  NAV_DATA_DIR="$dir" \
  NAV_LAN_BASE="$lan_base" \
    node "e2e/$script" || FAILED=1

  kill "$srv" 2>/dev/null || true
  wait "$srv" 2>/dev/null || true
}

run_case basic    "$BASE_PORT"         smoke.mjs
run_case folders  "$((BASE_PORT + 1))" folders.mjs
run_case pages    "$((BASE_PORT + 2))" pages.mjs
# 图标用例用本地站点做目标，需要放开私网抓取（默认是关的，见 README）
run_case icons    "$((BASE_PORT + 3))" icons.mjs NAV_ALLOW_PRIVATE_FETCH=1
run_case settings "$((BASE_PORT + 4))" settings.mjs
run_case transfer "$((BASE_PORT + 5))" transfer.mjs
run_case pwa      "$((BASE_PORT + 6))" pwa.mjs --addr 0.0.0.0

echo
if [ "$FAILED" -eq 0 ]; then echo "全部用例通过"; else echo "有用例失败"; fi
exit "$FAILED"
