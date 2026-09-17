#!/usr/bin/env bash
# 等 ghcr 上的镜像构建完成：匿名 manifest 查询（不需要 docker / gh 登录）。
#
# 用法：
#   deploy/wait-image.sh [tag]
#
#   tag 缺省时从 deploy/compose.yaml 的 `image:` 里取，所以发布流程通常是：
#     git tag v0.1.3 && git push --tags      # 触发 release.yml 的镜像任务
#     deploy/wait-image.sh                   # 等 0.1.3 的 manifest 就绪
#     ssh … 'cd /opt/1panel/docker/compose/my_nav && docker compose pull && docker compose up -d'
#
# 每 20s 查一次，最多 40 次（约 13 分钟）；每 5 轮顺带报一次 Actions 状态，
# 用来区分"还在构建"和"构建失败"。退出码 0=就绪，1=超时。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REPO="ffdkj/my_nav"

if [ "${1:-}" != "" ]; then
  TAG="$1"
else
  # 从 compose 文件里取当前部署的 tag（两个文件任选其一）
  TAG="$(sed -n 's|.*image: *ghcr.io/'"${REPO}"':\(.*\)$|\1|p' \
    "$ROOT/deploy/compose.yaml" "$ROOT/deploy/compose.host-network.yaml" 2>/dev/null | head -1)"
fi
if [ -z "${TAG:-}" ]; then
  echo "用法：$0 [tag]（没能从 deploy/compose.yaml 里推出 tag）" >&2
  exit 2
fi
echo "等待 ghcr.io/${REPO}:${TAG} …"

TMPD="$(mktemp -d)"
trap 'rm -rf "$TMPD"' EXIT

manifest_ok() {
  local tok
  tok=$(curl -fsS "https://ghcr.io/token?scope=repository:${REPO}:pull&service=ghcr.io" 2>/dev/null | sed -n 's/.*"token":"\([^"]*\)".*/\1/p')
  [ -n "$tok" ] || return 1
  curl -fsS -o "$TMPD/m.json" -w '%{http_code}' \
    -H "Authorization: Bearer $tok" \
    -H 'Accept: application/vnd.oci.image.index.v1+json,application/vnd.docker.distribution.manifest.list.v2+json' \
    "https://ghcr.io/v2/${REPO}/manifests/${TAG}" 2>/dev/null
}

actions_state() {
  curl -fsS "https://api.github.com/repos/${REPO}/actions/runs?per_page=3" 2>/dev/null |
    python3 -c "
import json,sys
try: d=json.load(sys.stdin)
except Exception: print('  (api 读不到)'); sys.exit()
for r in d.get('workflow_runs',[])[:3]:
    print(f\"  {r['head_branch']:<8} {r['event']:<6} {r['status']:<12} {r.get('conclusion') or '-':<10} {r['head_sha'][:7]} {r['created_at']}\")
"
}

for i in $(seq 1 40); do
  code=$(manifest_ok)
  if [ "$code" = "200" ]; then
    echo "READY: ghcr.io/${REPO}:${TAG} 已存在（第 ${i} 次查询）"
    python3 -c "
import json
d=json.load(open('${TMPD}/m.json'))
print('mediaType:', d.get('mediaType'))
for m in d.get('manifests',[]):
    p=m.get('platform',{})
    print(' ', p.get('os'), p.get('architecture'), m.get('size'), m.get('digest','')[:20])
" 2>/dev/null
    exit 0
  fi
  echo "[$i] tag=${TAG} manifest=http(${code:-fail}) $(date +%H:%M:%S)"
  if [ $((i % 5)) -eq 0 ]; then actions_state; fi
  sleep 20
done
echo "TIMEOUT: 10 分钟内没等到 ${TAG}"
exit 1
