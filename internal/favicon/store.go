package favicon

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Store 是**内容寻址**的图标仓库：
// 路径 = <sha256 前 2 位>/<sha256>.<ext>，因此同一个图标天然去重，
// 也让 /icons/... 可以安全地设成 immutable 缓存（路径变了内容一定变）。
type Store struct {
	Dir string
}

var ErrBadPath = errors.New("favicon: illegal icon path")

func NewStore(dir string) *Store { return &Store{Dir: dir} }

// Save 写入并返回相对路径（相对 Store.Dir）。
func (s *Store) Save(data []byte, mime string) (string, error) {
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	rel := filepath.Join(digest[:2], digest+"."+ExtFor(mime))
	full := filepath.Join(s.Dir, rel)

	if _, err := os.Stat(full); err == nil {
		return rel, nil // 同内容已存在，直接复用
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return "", fmt.Errorf("favicon: mkdir: %w", err)
	}

	// 先写临时文件再 rename：避免读到写了一半的图标
	tmp, err := os.CreateTemp(filepath.Dir(full), ".tmp-*")
	if err != nil {
		return "", fmt.Errorf("favicon: temp file: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("favicon: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("favicon: close: %w", err)
	}
	if err := os.Rename(tmpName, full); err != nil {
		_ = os.Remove(tmpName)
		return "", fmt.Errorf("favicon: rename: %w", err)
	}
	return rel, nil
}

// Resolve 把相对路径还原成绝对路径，并挡住目录穿越。
//
// 这里选择**显式拒绝**含 ".." 的路径，而不是静默清洗掉它：
// 清洗虽然同样安全，但会让 /icons/../../x 变成一个语义不明的请求，
// 失败要失败得清清楚楚。
func (s *Store) Resolve(rel string) (string, error) {
	// 空路径、NUL、绝对路径一律拒绝：调用方只会传 <hash 前两位>/<hash>.<ext>
	if rel == "" || strings.ContainsRune(rel, 0) || strings.HasPrefix(rel, "/") || strings.HasPrefix(rel, `\`) {
		return "", ErrBadPath
	}
	for _, part := range strings.FieldsFunc(rel, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return "", ErrBadPath
		}
	}
	clean := strings.TrimPrefix(filepath.Clean("/"+rel), "/")
	if clean == "" || clean == "." {
		return "", ErrBadPath
	}
	full := filepath.Join(s.Dir, clean)
	// 解析后必须仍在 Store.Dir 之内（symbolic link 也一并处理）
	absDir, err := filepath.Abs(s.Dir)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if absFull != absDir && !strings.HasPrefix(absFull, absDir+string(os.PathSeparator)) {
		return "", ErrBadPath
	}
	return absFull, nil
}
