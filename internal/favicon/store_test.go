package favicon

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreIsContentAddressed(t *testing.T) {
	store := NewStore(t.TempDir())
	data := makePNG(t, 32)

	first, err := store.Save(data, "image/png")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	second, err := store.Save(data, "image/png")
	if err != nil {
		t.Fatalf("Save again: %v", err)
	}
	if first != second {
		t.Errorf("同内容两次写入路径不同：%s vs %s", first, second)
	}
	if !strings.HasSuffix(first, ".png") || len(strings.Split(first, string(os.PathSeparator))) != 2 {
		t.Errorf("路径形状不对：%s", first)
	}

	// 内容不同 → 路径不同
	other, err := store.Save(makePNG(t, 33), "image/png")
	if err != nil {
		t.Fatalf("Save other: %v", err)
	}
	if other == first {
		t.Error("不同内容不该映射到同一路径")
	}

	full, err := store.Resolve(first)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if _, err := os.Stat(full); err != nil {
		t.Errorf("文件不存在：%v", err)
	}
}

func TestStoreResolveBlocksTraversal(t *testing.T) {
	store := NewStore(t.TempDir())
	for _, evil := range []string{
		"../../etc/passwd",
		"../secret.png",
		"/etc/passwd",
		"a/../../b.png",
		"",
	} {
		if _, err := store.Resolve(evil); !errors.Is(err, ErrBadPath) {
			t.Errorf("Resolve(%q) err = %v, want ErrBadPath", evil, err)
		}
	}
	// 正常相对路径要能解析
	rel := filepath.Join("ab", "abcdef.png")
	full, err := store.Resolve(rel)
	if err != nil {
		t.Fatalf("Resolve(%q): %v", rel, err)
	}
	if !strings.HasSuffix(full, filepath.Join("ab", "abcdef.png")) {
		t.Errorf("解析结果异常：%s", full)
	}
}

func TestExtFor(t *testing.T) {
	cases := map[string]string{
		"image/png":                "png",
		"image/x-icon":             "ico",
		"image/svg+xml":            "svg",
		"image/webp":               "webp",
		"application/octet-stream": "bin",
	}
	for mime, want := range cases {
		if got := ExtFor(mime); got != want {
			t.Errorf("ExtFor(%s) = %s, want %s", mime, got, want)
		}
	}
}
