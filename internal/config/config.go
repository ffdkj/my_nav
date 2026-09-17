// Package config 从环境变量读取运行配置。
// 设计原则：所有可调项都有安全的默认值，容器里只注入必要的几个（见 deploy/compose.yaml）。
package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	// Addr 是 HTTP 监听地址。容器内默认 :8080（对外由 Tailscale 虚拟 IP 发布）。
	Addr string
	// DataDir 是唯一的数据卷挂载点：nav.db / icons/ / wallpapers/ / backup/。
	DataDir string
	// LogLevel 映射到 slog 级别（debug|info|warn|error）。
	LogLevel slog.Level
	// DevMode 打开后允许更宽松的日志与调试端点。
	DevMode bool
}

func Load() Config {
	cfg := Config{
		Addr:     env("NAV_ADDR", ":8080"),
		DataDir:  env("NAV_DATA_DIR", "./data"),
		LogLevel: parseLevel(env("NAV_LOG_LEVEL", "info")),
		DevMode:  env("NAV_DEV", "") != "",
	}
	return cfg
}

// DBPath 返回 SQLite 文件路径。
func (c Config) DBPath() string { return filepath.Join(c.DataDir, "nav.db") }

// IconsDir / WallpapersDir / BackupDir 是三个子目录的绝对路径。
func (c Config) IconsDir() string      { return filepath.Join(c.DataDir, "icons") }
func (c Config) WallpapersDir() string { return filepath.Join(c.DataDir, "wallpapers") }
func (c Config) BackupDir() string     { return filepath.Join(c.DataDir, "backup") }

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
