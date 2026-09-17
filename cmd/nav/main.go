// Command nav 是 my_nav 的单一二进制入口：
// 打开 SQLite → 跑迁移 → 装配路由 → 监听，收到信号后优雅退出。
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ffdkj/my_nav/internal/config"
	"github.com/ffdkj/my_nav/internal/db"
	"github.com/ffdkj/my_nav/internal/favicon"
	"github.com/ffdkj/my_nav/internal/migrate"
	"github.com/ffdkj/my_nav/internal/nav"
	"github.com/ffdkj/my_nav/internal/server"
)

func main() {
	// distroless 镜像里没有 shell / curl，Dockerfile 的 HEALTHCHECK 直接调用二进制自身。
	if len(os.Args) > 1 && os.Args[1] == "-healthcheck" {
		if err := healthcheck(); err != nil {
			fmt.Fprintln(os.Stderr, "healthcheck:", err)
			os.Exit(1)
		}
		return
	}

	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// healthcheck 探测本进程的 /healthz；监听地址取自 NAV_ADDR。
func healthcheck() error {
	addr := config.Load().Addr
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("parse NAV_ADDR %q: %w", addr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://" + net.JoinHostPort(host, port) + "/healthz")
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return nil
}

func run() error {
	cfg := config.Load()
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel})))

	for _, dir := range []string{cfg.DataDir, cfg.IconsDir(), cfg.WallpapersDir(), cfg.BackupDir()} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	handle, err := db.Open(cfg.DBPath())
	if err != nil {
		return err
	}
	defer func() { _ = handle.Close() }()

	version, err := migrate.Run(ctx, handle, slog.Default())
	if err != nil {
		return err
	}
	slog.Info("database ready", "path", cfg.DBPath(), "schema_version", version)

	srv := &http.Server{
		Addr: cfg.Addr,
		Handler: server.New(cfg, nav.New(handle,
			nav.WithIcons(favicon.NewStore(cfg.IconsDir()), favicon.NewFetcher(slog.Default(), cfg.AllowPrivateFetch)),
			nav.WithLogger(slog.Default()),
		)),
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Addr, "data_dir", cfg.DataDir)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
