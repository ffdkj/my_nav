// Package imageproc 处理壁纸图片：校验、缩放、生成缩略图。
//
// 为什么缩略图用 JPEG 而不是 WebP：
// Go 的 golang.org/x/image 只有 WebP **解码**，没有编码器；纯 Go 的 WebP 编码
// 需要引入 cgo 或第三方实现，会把 CGO_ENABLED=0 的静态单二进制优势赔进去。
// 壁纸是照片类内容，JPEG q82 在 1920 宽下的体积与质量都够用。
package imageproc

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
)

// MaxWallpaperBytes 是壁纸文件上限（spec：≤10MB）。
const MaxWallpaperBytes = 10 << 20

var ErrUnsupportedImage = errors.New("imageproc: unsupported or corrupt image")

// Thumbnail 解码图片并生成不超过 maxWidth 宽的 JPEG 缩略图（保持比例）。
func Thumbnail(data []byte, maxWidth, quality int) (out []byte, w, h int, err error) {
	if len(data) == 0 {
		return nil, 0, 0, ErrUnsupportedImage
	}
	if len(data) > MaxWallpaperBytes {
		return nil, 0, 0, fmt.Errorf("imageproc: %d bytes exceeds the %d limit", len(data), MaxWallpaperBytes)
	}

	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, 0, ErrUnsupportedImage
	}
	bounds := src.Bounds()
	sw, sh := bounds.Dx(), bounds.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, 0, 0, ErrUnsupportedImage
	}

	tw, th := sw, sh
	if sw > maxWidth {
		tw = maxWidth
		th = int(float64(sh) * float64(maxWidth) / float64(sw))
	}
	if th < 1 {
		th = 1
	}

	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, 0, 0, err
	}
	return buf.Bytes(), tw, th, nil
}

// Dimensions 只读取尺寸，用于登记原始文件。
func Dimensions(data []byte) (w, h int, err error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return 0, 0, ErrUnsupportedImage
	}
	return cfg.Width, cfg.Height, nil
}

// Format 返回图片的 MIME（以解码器识别结果为准，不受图标那 2MiB 上限影响）。
func Format(data []byte) (string, error) {
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", ErrUnsupportedImage
	}
	switch format {
	case "png":
		return "image/png", nil
	case "jpeg":
		return "image/jpeg", nil
	case "gif":
		return "image/gif", nil
	case "webp":
		return "image/webp", nil
	default:
		return "", ErrUnsupportedImage
	}
}
