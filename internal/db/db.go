// Package db 负责打开 SQLite 连接。
//
// 关键约束（来自 docs/research/02-backend-go-sqlc-docker.md，已实测）：
//   - 驱动名是 "sqlite"（modernc.org/sqlite），纯 Go，CGO_ENABLED=0 可用。
//   - busy_timeout / foreign_keys / synchronous 是 **逐连接** 生效的，
//     必须写进 DSN；用 db.Exec("PRAGMA ...") 只会配置当时那一条连接。
//   - 单用户应用用 SetMaxOpenConns(1) 串行化写入，避免 SQLITE_BUSY。
package db

import (
	"database/sql"
	"fmt"
	"net/url"

	_ "modernc.org/sqlite"
)

// DSN 组装 modernc 的 DSN（_pragma=name(value) 可重复）。
func DSN(path string) string {
	q := url.Values{}
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "synchronous(NORMAL)")
	return "file:" + path + "?" + q.Encode()
}

// Open 打开数据库并做必要的连接池设置。
func Open(path string) (*sql.DB, error) {
	handle, err := sql.Open("sqlite", DSN(path))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// 单写入者模型；读也走同一条连接，对本项目的数据量完全够用。
	handle.SetMaxOpenConns(1)
	handle.SetMaxIdleConns(1)

	if err := handle.Ping(); err != nil {
		_ = handle.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return handle, nil
}
