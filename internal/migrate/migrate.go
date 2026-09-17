// Package migrate 是一个零依赖的迁移器：
// embed 目录里的 *.sql 按文件名排序执行，版本号存在 PRAGMA user_version。
//
// 约定：
//   - 文件名必须零填充（001_、002_…），因为排序是词法的。
//   - 一个文件 = 一个版本，整体在单事务内执行。
//   - PRAGMA user_version 不接受绑定参数，只能插值（数字已用 Atoi 校验）。
package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"path"
	"sort"
	"strconv"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Run 把所有未应用的迁移按序执行，返回最终版本号。
func Run(ctx context.Context, handle *sql.DB, logger *slog.Logger) (int, error) {
	entries, err := fs.Glob(migrationsFS, "migrations/*.sql")
	if err != nil {
		return 0, fmt.Errorf("glob migrations: %w", err)
	}
	if len(entries) == 0 {
		return 0, fmt.Errorf("no migrations embedded")
	}
	sort.Strings(entries)

	var current int
	if err := handle.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&current); err != nil {
		return current, fmt.Errorf("read user_version: %w", err)
	}

	for _, name := range entries {
		version, err := parseVersion(name)
		if err != nil {
			return current, err
		}
		if version <= current {
			continue
		}

		body, err := migrationsFS.ReadFile(name)
		if err != nil {
			return current, fmt.Errorf("read %s: %w", name, err)
		}

		tx, err := handle.BeginTx(ctx, nil)
		if err != nil {
			return current, fmt.Errorf("begin tx for %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return current, fmt.Errorf("apply %s: %w", name, err)
		}
		// 版本号来自文件名且已用 Atoi 校验，插值是安全的。
		if _, err := tx.ExecContext(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, version)); err != nil {
			_ = tx.Rollback()
			return current, fmt.Errorf("set user_version=%d: %w", version, err)
		}
		if err := tx.Commit(); err != nil {
			return current, fmt.Errorf("commit %s: %w", name, err)
		}

		current = version
		if logger != nil {
			logger.Info("migration applied", "file", path.Base(name), "version", version)
		}
	}
	return current, nil
}

func parseVersion(name string) (int, error) {
	base := path.Base(name)
	head, _, ok := strings.Cut(base, "_")
	if !ok {
		return 0, fmt.Errorf("migration %q must be named like 001_init.sql", base)
	}
	version, err := strconv.Atoi(head)
	if err != nil {
		return 0, fmt.Errorf("migration %q has non-numeric version prefix: %w", base, err)
	}
	return version, nil
}
