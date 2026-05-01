//go:build ignore

package main

import (
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
)

const (
	iconSize = 1024
	scale    = 4
)

type rgba struct {
	r float64
	g float64
	b float64
	a float64
}

// main 生成应用图标 PNG，供 Wails 在 macOS 打包时转换为 Dock 图标。
func main() {
	source := renderIcon(iconSize * scale)
	icon := downsample(source, iconSize)
	if err := os.MkdirAll("build", 0o755); err != nil {
		panic(err)
	}
	if err := writePNG(filepath.Join("build", "appicon.png"), icon); err != nil {
		panic(err)
	}
}

// renderIcon 使用高分辨率画布绘制图标，再通过降采样获得平滑边缘。
func renderIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			px := float64(x) / float64(size-1) * iconSize
			py := float64(y) / float64(size-1) * iconSize
			if !insideRoundedRect(px, py, 94, 94, 836, 836, 132) {
				continue
			}

			bg := backgroundColor(px, py)
			if insideCloud(px, py) {
				bg = rgba{r: 255, g: 255, b: 255, a: 255}
			}
			if insideTunnelLine(px, py) || insideCircle(px, py, 390, 610, 48) || insideCircle(px, py, 515, 532, 48) || insideCircle(px, py, 640, 610, 48) {
				bg = rgba{r: 37, g: 99, b: 235, a: 255}
			}
			if insideCircle(px, py, 390, 610, 18) || insideCircle(px, py, 515, 532, 18) || insideCircle(px, py, 640, 610, 18) {
				bg = rgba{r: 255, g: 255, b: 255, a: 255}
			}
			img.SetNRGBA(x, y, toNRGBA(bg))
		}
	}
	return img
}

// backgroundColor 生成橙色渐变背景，让图标在 Dock 上更容易识别。
func backgroundColor(x float64, y float64) rgba {
	t := y / iconSize
	top := rgba{r: 255, g: 157, b: 64, a: 255}
	bottom := rgba{r: 232, g: 87, b: 31, a: 255}
	highlight := math.Max(0, 1-(x+y)/(iconSize*1.05)) * 18
	return rgba{
		r: mix(top.r, bottom.r, t) + highlight,
		g: mix(top.g, bottom.g, t) + highlight*0.55,
		b: mix(top.b, bottom.b, t),
		a: 255,
	}
}

// insideCloud 判断坐标是否处于主云朵区域。
func insideCloud(x float64, y float64) bool {
	return insideCircle(x, y, 368, 560, 150) ||
		insideCircle(x, y, 515, 482, 192) ||
		insideCircle(x, y, 672, 565, 145) ||
		insideRoundedRect(x, y, 270, 525, 505, 190, 92)
}

// insideTunnelLine 判断坐标是否处于连接节点的通道线区域。
func insideTunnelLine(x float64, y float64) bool {
	return distanceToSegment(x, y, 390, 610, 515, 532) <= 28 ||
		distanceToSegment(x, y, 515, 532, 640, 610) <= 28
}

// insideRoundedRect 判断坐标是否落在圆角矩形内。
func insideRoundedRect(x float64, y float64, left float64, top float64, width float64, height float64, radius float64) bool {
	right := left + width
	bottom := top + height
	if x < left || x > right || y < top || y > bottom {
		return false
	}
	cx := math.Min(math.Max(x, left+radius), right-radius)
	cy := math.Min(math.Max(y, top+radius), bottom-radius)
	return math.Hypot(x-cx, y-cy) <= radius
}

// insideCircle 判断坐标是否落在圆形内。
func insideCircle(x float64, y float64, cx float64, cy float64, radius float64) bool {
	return math.Hypot(x-cx, y-cy) <= radius
}

// distanceToSegment 计算点到线段的距离，用于绘制圆角线条。
func distanceToSegment(px float64, py float64, ax float64, ay float64, bx float64, by float64) float64 {
	dx := bx - ax
	dy := by - ay
	if dx == 0 && dy == 0 {
		return math.Hypot(px-ax, py-ay)
	}
	t := ((px-ax)*dx + (py-ay)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(ax+t*dx), py-(ay+t*dy))
}

// downsample 对高分辨率图像做平均降采样，减少锯齿。
func downsample(src *image.NRGBA, targetSize int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, targetSize, targetSize))
	step := src.Bounds().Dx() / targetSize
	for y := 0; y < targetSize; y++ {
		for x := 0; x < targetSize; x++ {
			var r, g, b, a uint32
			for yy := 0; yy < step; yy++ {
				for xx := 0; xx < step; xx++ {
					c := src.NRGBAAt(x*step+xx, y*step+yy)
					r += uint32(c.R)
					g += uint32(c.G)
					b += uint32(c.B)
					a += uint32(c.A)
				}
			}
			count := uint32(step * step)
			dst.SetNRGBA(x, y, color.NRGBA{R: uint8(r / count), G: uint8(g / count), B: uint8(b / count), A: uint8(a / count)})
		}
	}
	return dst
}

// writePNG 将生成结果写入指定路径。
func writePNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, img)
}

// mix 做线性插值，用于背景渐变。
func mix(a float64, b float64, t float64) float64 {
	return a + (b-a)*t
}

// toNRGBA 将浮点颜色安全转换成 PNG 可写入的颜色。
func toNRGBA(c rgba) color.NRGBA {
	return color.NRGBA{
		R: uint8(math.Max(0, math.Min(255, c.r))),
		G: uint8(math.Max(0, math.Min(255, c.g))),
		B: uint8(math.Max(0, math.Min(255, c.b))),
		A: uint8(math.Max(0, math.Min(255, c.a))),
	}
}
