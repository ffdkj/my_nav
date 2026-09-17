#!/usr/bin/env bash
#
# my_nav 一键部署（在目标主机上以 root 执行）
#
#   bash install.sh                 # 用默认值部署
#   NAV_BIND_IP=127.0.0.1 bash install.sh
#   IMAGE=ghcr.io/ffdkj/my_nav:edge bash install.sh
#
# 幂等：重复执行只会拉取/重建容器，不会覆盖已有的 .env 与 data/。
set -euo pipefail

VERSION="${VERSION:-0.1.0}"
IMAGE="${IMAGE:-ghcr.io/ffdkj/my_nav:${VERSION}}"
APP_DIR="${APP_DIR:-/opt/1panel/docker/compose/my_nav}"
NAV_BIND_IP="${NAV_BIND_IP:-100.70.0.29}"
NAV_PORT="${NAV_PORT:-8090}"
RAW_BASE="${RAW_BASE:-https://raw.githubusercontent.com/ffdkj/my_nav/v${VERSION}/deploy}"

say() { printf '\033[36m==>\033[0m %s\n' "$*"; }
die() { printf '\033[31m错误:\033[0m %s\n' "$*" >&2; exit 1; }

# ---------- 1. 前置检查 ----------
say "检查运行环境"
command -v docker >/dev/null 2>&1 || die "未找到 docker"
docker info >/dev/null 2>&1 || die "docker 守护进程不可用（当前用户有没有权限？）"

if docker compose version >/dev/null 2>&1; then
  DC=(docker compose)
elif command -v docker-compose >/dev/null 2>&1; then
  DC=(docker-compose)
  say "提示：没有 docker compose 插件，将使用独立的 docker-compose"
else
  die "既没有 'docker compose' 子命令，也没有独立的 docker-compose"
fi
say "使用：${DC[*]}"

# 抓图标要出网；这里只提示，不阻断部署
if ! curl -sS -o /dev/null --max-time 8 "https://t2.gstatic.com/faviconV2?client=SOCIAL&type=FAVICON&fallback_opts=TYPE,SIZE,URL&url=https://github.com&size=64"; then
  say "警告：抓不到 Google favicon 端点，图标抓取会全部走兜底（纯色文字）"
fi

# ---------- 2. 目录与配置 ----------
say "准备目录 $APP_DIR"
mkdir -p "$APP_DIR/data"

if [ ! -f "$APP_DIR/compose.yaml" ]; then
  say "下载 compose.yaml"
  curl -fsSL "$RAW_BASE/compose.yaml" -o "$APP_DIR/compose.yaml" \
    || die "下载 compose.yaml 失败（检查出网）"
fi

if [ ! -f "$APP_DIR/.env" ]; then
  say "写入 .env（绑定 $NAV_BIND_IP:$NAV_PORT）"
  cat > "$APP_DIR/.env" <<EOF
# 由 install.sh 生成。改完记得：${DC[*]} up -d
# - 100.70.0.29 = 本机 Tailscale 虚拟 IP（其他 tailnet 设备可访问）
# - 127.0.0.1    = 仅本机（配合 tailscale serve 出 HTTPS 时用这个）
NAV_BIND_IP=$NAV_BIND_IP
NAV_PORT=$NAV_PORT
NAV_LOG_LEVEL=info
EOF
else
  say ".env 已存在，保持不变（当前绑定：$(grep -E '^NAV_BIND_IP' "$APP_DIR/.env" || echo '未设置'))"
fi

# ---------- 3. 拉镜像并启动 ----------
say "拉取镜像 $IMAGE"
docker pull "$IMAGE" || die "拉取失败（私有仓库？网络？）"

say "启动"
cd "$APP_DIR"
"${DC[@]}" up -d

# ---------- 4. 健康检查 ----------
say "等待健康检查"
for _ in $(seq 1 30); do
  status="$(docker inspect --format '{{.State.Health.Status}}' my_nav 2>/dev/null || echo unknown)"
  [ "$status" = "healthy" ] && break
  sleep 1
done
status="$(docker inspect --format '{{.State.Health.Status}}' my_nav 2>/dev/null || echo unknown)"

# ---------- 5. 结果 ----------
BOUND_IP="$(grep -E '^NAV_BIND_IP' "$APP_DIR/.env" | cut -d= -f2)"
BOUND_PORT="$(grep -E '^NAV_PORT' "$APP_DIR/.env" | cut -d= -f2)"
echo
say "容器状态：$status"
say "访问地址：http://${BOUND_IP}:${BOUND_PORT}"
echo
cat <<'NEXT'
接下来建议手动确认：

  1) 浏览器打开上面的地址，应该能看到网格与「+ 添加」图块
  2) 添加一个网址（例如 https://github.com），几秒内应出现真实图标；
     若显示纯色字母，说明出网受限，可用编辑面板里的「重新抓取」或上传本地图标
  3) 手机上打开同一地址，试一次拖拽（按住 300ms 再拖）
  4) 想看日志：docker logs -f my_nav
  5) 想装成手机应用（PWA）需要 HTTPS，见 docs/deploy.md 的「可选：启用 PWA」
NEXT
