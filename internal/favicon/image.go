package favicon

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // 注册 webp 解码器（仅解码）

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

const (
	// maxIconBytes 是单个图标的字节上限。
	maxIconBytes = 2 << 20 // 2 MiB
	// maxIconPixels 防解压炸弹：一个很小的文件也能声明巨大的画布。
	maxIconPixels = 4096 * 4096
)

var ErrUnknownImage = errors.New("favicon: unrecognized image format")

// readCapped 读取响应体，超过上限即报错（注意要在状态码校验之后调用）。
func readCapped(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("favicon: body exceeds %d bytes", limit)
	}
	return data, nil
}

// Inspect 判断字节流是不是一个可用的图标，并给出 MIME 与尺寸。
//
// ⚠️ http.DetectContentType 只看前 512 字节，而且**不认识 ICO**，
// 所以它只能当粗筛；真正的依据是能否 DecodeConfig。
// ICO / SVG 不做解码（我们不引入 ICO 解码库），只做魔数/标记判断后原样存盘。
func Inspect(data []byte) (mime string, w, h int, err error) {
	if len(data) == 0 {
		return "", 0, 0, ErrUnknownImage
	}
	if len(data) > maxIconBytes {
		return "", 0, 0, fmt.Errorf("favicon: %d bytes exceeds the %d limit", len(data), maxIconBytes)
	}

	// ICO: 00 00 01 00
	if len(data) >= 4 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x01 && data[3] == 0x00 {
		return "image/x-icon", 0, 0, nil
	}
	// SVG: 文本，允许前置 BOM / 空白 / XML 声明
	if looksLikeSVG(data) {
		return "image/svg+xml", 0, 0, nil
	}

	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", 0, 0, ErrUnknownImage
	}
	if cfg.Width*cfg.Height > maxIconPixels {
		return "", 0, 0, fmt.Errorf("favicon: %dx%d exceeds the pixel budget", cfg.Width, cfg.Height)
	}
	switch format {
	case "png":
		return "image/png", cfg.Width, cfg.Height, nil
	case "jpeg":
		return "image/jpeg", cfg.Width, cfg.Height, nil
	case "gif":
		return "image/gif", cfg.Width, cfg.Height, nil
	case "webp":
		return "image/webp", cfg.Width, cfg.Height, nil
	default:
		return "", 0, 0, ErrUnknownImage
	}
}

func looksLikeSVG(data []byte) bool {
	head := data
	if len(head) > 1024 {
		head = head[:1024]
	}
	// 去掉 BOM 与前导空白
	head = bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF})
	head = bytes.TrimLeft(head, " \t\r\n")
	if bytes.HasPrefix(head, []byte("<?xml")) {
		if i := bytes.Index(head, []byte("<svg")); i >= 0 {
			return true
		}
		return false
	}
	return bytes.HasPrefix(head, []byte("<svg"))
}

// ExtFor 返回内容寻址存储用的扩展名。
func ExtFor(mime string) string {
	switch mime {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	case "image/gif":
		return "gif"
	case "image/webp":
		return "webp"
	case "image/x-icon":
		return "ico"
	case "image/svg+xml":
		return "svg"
	default:
		return "bin"
	}
}

// NormalizeUpload 把用户上传的图片统一成边长不超过 max 的 PNG。
// SVG 原样返回（它是矢量的，缩放在前端做）。
func NormalizeUpload(data []byte, max int) (out []byte, mime string, w, h int, err error) {
	mime, w, h, err = Inspect(data)
	if err != nil {
		return nil, "", 0, 0, err
	}
	if mime == "image/svg+xml" || mime == "image/x-icon" {
		return data, mime, w, h, nil
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, "", 0, 0, ErrUnknownImage
	}
	bounds := src.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()
	if sw <= max && sh <= max {
		out = data
	} else {
		scale := float64(max) / float64(sw)
		if sh > sw {
			scale = float64(max) / float64(sh)
		}
		nw, nh := int(float64(sw)*scale), int(float64(sh)*scale)
		if nw < 1 {
			nw = 1
		}
		if nh < 1 {
			nh = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)
		var buf bytes.Buffer
		if err := png.Encode(&buf, dst); err != nil {
			return nil, "", 0, 0, err
		}
		out, w, h = buf.Bytes(), nw, nh
	}

	var buf bytes.Buffer
	img, format, err := image.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, "", 0, 0, ErrUnknownImage
	}
	if format != "png" {
		if err := png.Encode(&buf, img); err != nil {
			return nil, "", 0, 0, err
		}
		out = buf.Bytes()
	}
	return out, "image/png", w, h, nil
}

// 保留编码器引用，避免被 lint 判为未使用（上传归一化只用 png）。
var (
	_ = jpeg.Encode
	_ = gif.Encode
	_ = http.DetectContentType
)
