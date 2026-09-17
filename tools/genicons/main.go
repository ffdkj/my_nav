// Command genicons 生成 PWA 图标（192/512）。
//
// 为什么自己画而不是引图标库：只要两个纯色底 + 一个字形，
// 用 stdlib 的 image/draw 三十行就够了，不值得为它增加依赖与构建步骤。
// 产物入库（web/public/pwa-*.png），不需要在 CI/构建期重跑。
//
// 用法：go run ./tools/genicons
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

var (
	background = color.RGBA{R: 0x0d, G: 0x14, B: 0x24, A: 0xff}
	accent     = color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 0xff}
)

// drawN 在 (0..1) 归一化坐标里画一个 "N"：左右两根竖条 + 一条斜杠。
func drawN(img *image.RGBA, size int, scale float64) {
	px := func(v float64) int { return int(v * float64(size)) }

	barW := px(0.11 * scale)
	top, bottom := px(0.5-0.26*scale), px(0.5+0.26*scale)
	left, right := px(0.5-0.22*scale), px(0.5+0.22*scale)

	fill := func(x0, y0, x1, y1 int) {
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				if x >= 0 && y >= 0 && x < size && y < size {
					img.Set(x, y, accent)
				}
			}
		}
	}

	fill(left, top, left+barW, bottom)
	fill(right-barW, top, right, bottom)

	// 斜杠：沿对角线摆一串小方块，保证顶部连到左条、底部连到右条
	steps := 220
	thick := barW
	for i := 0; i <= steps; i++ {
		t := float64(i) / float64(steps)
		cx := float64(left) + t*float64(right-barW-left)
		cy := float64(top) + t*float64(bottom-top-thick)
		fill(int(cx), int(cy), int(cx)+thick, int(cy)+thick)
	}
}

func roundedRect(img *image.RGBA, size int, radius float64) {
	r := radius * float64(size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			fx, fy := float64(x)+0.5, float64(y)+0.5
			// 四角做圆角裁切
			var dx, dy float64
			switch {
			case fx < r && fy < r:
				dx, dy = r-fx, r-fy
			case fx > float64(size)-r && fy < r:
				dx, dy = fx-(float64(size)-r), r-fy
			case fx < r && fy > float64(size)-r:
				dx, dy = r-fx, fy-(float64(size)-r)
			case fx > float64(size)-r && fy > float64(size)-r:
				dx, dy = fx-(float64(size)-r), fy-(float64(size)-r)
			default:
				img.Set(x, y, background)
				continue
			}
			if math.Hypot(dx, dy) <= r {
				img.Set(x, y, background)
			}
		}
	}
}

func main() {
	out := "web/public"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		log.Fatal(err)
	}

	// maskable 图标需要留安全边距（内容不超过 80%），所以字形缩小一档
	for _, spec := range []struct {
		name   string
		size   int
		scale  float64
		radius float64
	}{
		{"pwa-192x192.png", 192, 1.0, 0.22},
		{"pwa-512x512.png", 512, 1.0, 0.22},
		{"pwa-maskable-512x512.png", 512, 0.8, 0.0},
		{"apple-touch-icon.png", 180, 1.0, 0.0},
	} {
		img := image.NewRGBA(image.Rect(0, 0, spec.size, spec.size))
		if spec.radius > 0 {
			roundedRect(img, spec.size, spec.radius)
		} else {
			// maskable / apple-touch 用整块背景（系统会自己裁切）
			for y := 0; y < spec.size; y++ {
				for x := 0; x < spec.size; x++ {
					img.Set(x, y, background)
				}
			}
		}
		drawN(img, spec.size, spec.scale)

		path := filepath.Join(out, spec.name)
		file, err := os.Create(path)
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(file, img); err != nil {
			log.Fatal(err)
		}
		_ = file.Close()
		fmt.Printf("wrote %s (%dx%d)\n", path, spec.size, spec.size)
	}
}
