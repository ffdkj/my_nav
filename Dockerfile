# syntax=docker/dockerfile:1

# ---------- 1. 前端：构建 SPA ----------
FROM node:22-bookworm-slim AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
# vite.config.ts 的 outDir 是 ../internal/web/dist，正好落在下面的 COPY 路径上
RUN npm run build

# ---------- 2. 后端：编译静态二进制（前端已内嵌） ----------
FROM golang:1.26-bookworm AS gobuild
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
# CGO_ENABLED=0 是关键：modernc.org/sqlite 是纯 Go 实现，因此能用 distroless/static
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/nav ./cmd/nav

# ---------- 3. 运行：最小镜像、非 root、只读根文件系统友好 ----------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=gobuild /out/nav /nav
# 唯一需要写入的路径：SQLite 库 + 图标/壁纸缓存 + 备份
VOLUME ["/data"]
EXPOSE 8080
ENV NAV_ADDR=:8080 NAV_DATA_DIR=/data
USER nonroot:nonroot
# distroless 里没有 shell/curl，健康检查用二进制自带的 -healthcheck 子命令
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/nav", "-healthcheck"]
ENTRYPOINT ["/nav"]
