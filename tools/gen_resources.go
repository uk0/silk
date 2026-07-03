//go:build ignore
// +build ignore

// gen_resources generates icon and theme PNG files for the Silk UI framework.
// Usage: go run tools/gen_resources.go [output_dir]
// Default output: current directory (for use alongside the binary)
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strconv"
)

func main() {
	outDir := "."
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	iconDir := filepath.Join(outDir, "icon")
	themeDir := filepath.Join(outDir, "theme", "default")
	os.MkdirAll(iconDir, 0755)
	os.MkdirAll(themeDir, 0755)

	icons := map[string]func(int) *image.RGBA{
		"document":           rectIcon(color.RGBA{100, 149, 237, 255}, color.RGBA{255, 255, 255, 255}),
		"folder":             rectIcon(color.RGBA{200, 170, 50, 255}, color.RGBA{255, 220, 100, 255}),
		"save":               rectIcon(color.RGBA{60, 100, 180, 255}, color.RGBA{100, 149, 237, 255}),
		"close":              crossIcon(color.RGBA{220, 80, 80, 255}),
		"close-btn":          crossIcon(color.RGBA{200, 60, 60, 255}),
		"exit":               crossIcon(color.RGBA{180, 50, 50, 255}),
		"file":               rectIcon(color.RGBA{150, 150, 150, 255}, color.RGBA{220, 220, 220, 255}),
		"edit-undo":          arrowIcon(color.RGBA{100, 149, 237, 255}),
		"edit-redo":          arrowIcon(color.RGBA{100, 149, 237, 255}),
		"preview":            rectIcon(color.RGBA{50, 150, 50, 255}, color.RGBA{100, 200, 100, 255}),
		"minimize":           lineIcon(color.RGBA{80, 80, 80, 255}),
		"maximize":           maxIcon(color.RGBA{80, 80, 80, 255}),
		"globe":              circleIcon(color.RGBA{50, 100, 200, 255}, color.RGBA{100, 149, 237, 255}),
		"error":              crossIcon(color.RGBA{220, 50, 50, 255}),
		"question":           circleIcon(color.RGBA{220, 230, 255, 255}, color.RGBA{100, 149, 237, 255}),
		"clipboard":          rectIcon(color.RGBA{240, 230, 210, 255}, color.RGBA{180, 160, 120, 255}),
		"form":               rectIcon(color.RGBA{230, 240, 255, 255}, color.RGBA{150, 180, 220, 255}),
		"silk-design":        circleIcon(color.RGBA{255, 255, 255, 255}, color.RGBA{100, 149, 237, 255}),
		"tree-view":          rectIcon(color.RGBA{80, 120, 180, 255}, color.RGBA{150, 180, 220, 255}),
		"diagram":            circleIcon(color.RGBA{80, 150, 80, 255}, color.RGBA{150, 200, 150, 255}),
		"map":                rectIcon(color.RGBA{50, 130, 50, 255}, color.RGBA{150, 200, 150, 255}),
		"arrow-tool":         arrowIcon(color.RGBA{60, 60, 60, 255}),
		"rect-tool":          rectIcon(color.RGBA{0, 0, 0, 0}, color.RGBA{100, 149, 237, 255}),
		"pencil":             rectIcon(color.RGBA{255, 200, 50, 255}, color.RGBA{80, 80, 80, 255}),
		"handle-0-normal":    handleIcon(color.RGBA{100, 149, 237, 255}, color.RGBA{255, 255, 255, 255}),
		"handle-1-normal":    handleIcon(color.RGBA{100, 200, 100, 255}, color.RGBA{255, 255, 255, 255}),
		"handle-2-normal":    handleIcon(color.RGBA{237, 149, 100, 255}, color.RGBA{255, 255, 255, 255}),
		"handle-0-active":    handleIcon(color.RGBA{50, 100, 200, 255}, color.RGBA{220, 230, 255, 255}),
		"handle-1-active":    handleIcon(color.RGBA{50, 160, 50, 255}, color.RGBA{220, 255, 220, 255}),
		"handle-2-active":    handleIcon(color.RGBA{200, 100, 50, 255}, color.RGBA{255, 230, 220, 255}),
		"checkbox-checked":   checkboxIcon(true),
		"checkbox-unchecked": checkboxIcon(false),
		"expander-collapsed": triRightIcon(color.RGBA{80, 80, 80, 255}),
		"expander-expanded":  triDownIcon(color.RGBA{80, 80, 80, 255}),
		"propsheet":          rectIcon(color.RGBA{245, 240, 230, 255}, color.RGBA{180, 160, 120, 255}),
		"window":             rectIcon(color.RGBA{200, 210, 230, 255}, color.RGBA{100, 130, 180, 255}),
		"edit":               rectIcon(color.RGBA{240, 240, 245, 255}, color.RGBA{100, 100, 120, 255}),
	}

	sizes := []int{16, 22, 32, 48}
	n := 0
	for name, gen := range icons {
		for _, sz := range sizes {
			save(filepath.Join(iconDir, name+"_"+strconv.Itoa(sz)+".png"), gen(sz))
			n++
		}
	}

	themes := [][4]interface{}{
		{"btn-active.png", 32, 32, color.RGBA{200, 210, 230, 255}},
		{"btn-pushed.png", 32, 32, color.RGBA{170, 185, 210, 255}},
		{"vscroll.png", 12, 32, color.RGBA{160, 170, 190, 255}},
		{"hscroll.png", 32, 12, color.RGBA{160, 170, 190, 255}},
		{"vscroll-track.png", 12, 32, color.RGBA{230, 230, 235, 255}},
		{"hscroll-track.png", 32, 12, color.RGBA{230, 230, 235, 255}},
		{"vscroll-track-active.png", 12, 32, color.RGBA{210, 215, 225, 255}},
		{"hscroll-track-active.png", 32, 12, color.RGBA{210, 215, 225, 255}},
		{"tab.png", 80, 28, color.RGBA{220, 225, 235, 255}},
		{"tab-hover.png", 80, 28, color.RGBA{200, 210, 230, 255}},
	}
	for _, t := range themes {
		save(filepath.Join(themeDir, t[0].(string)), themeImg(t[1].(int), t[2].(int), t[3].(color.RGBA)))
		n++
	}

	fmt.Printf("Generated %d resource files in %s/\n", n, outDir)
}

func save(p string, img *image.RGBA) { f, _ := os.Create(p); defer f.Close(); png.Encode(f, img) }
func imax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func rectIcon(fg, bg color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		m := imax(1, sz/8)
		for y := m; y < sz-m; y++ {
			for x := m; x < sz-m; x++ {
				if x == m || x == sz-m-1 || y == m || y == sz-m-1 {
					img.SetRGBA(x, y, fg)
				} else {
					img.SetRGBA(x, y, bg)
				}
			}
		}
		return img
	}
}
func circleIcon(bg, fg color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		cx, cy, r := float64(sz)/2, float64(sz)/2, float64(sz)/2-1
		for y := 0; y < sz; y++ {
			for x := 0; x < sz; x++ {
				d := math.Sqrt(float64((x-int(cx))*(x-int(cx)) + (y-int(cy))*(y-int(cy))))
				if d <= r-1 {
					img.SetRGBA(x, y, bg)
				} else if d <= r {
					img.SetRGBA(x, y, fg)
				}
			}
		}
		return img
	}
}
func crossIcon(c color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		m := sz / 4
		for i := 0; i < sz-2*m; i++ {
			for t := -1; t <= 1; t++ {
				if m+i+t >= 0 && m+i+t < sz {
					img.SetRGBA(m+i, m+i+t, c)
					img.SetRGBA(sz-1-m-i, m+i+t, c)
				}
			}
		}
		return img
	}
}
func arrowIcon(c color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		cx := sz * 2 / 3
		for y := sz / 4; y < sz*3/4; y++ {
			dy := y - sz/2
			if dy < 0 {
				dy = -dy
			}
			for x := cx - (sz/4 - dy); x <= cx && x < sz; x++ {
				if x >= 0 {
					img.SetRGBA(x, y, c)
				}
			}
		}
		return img
	}
}
func lineIcon(c color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		y := sz * 3 / 4
		t := imax(2, sz/8)
		for x := sz / 4; x < sz*3/4; x++ {
			for dy := 0; dy < t; dy++ {
				img.SetRGBA(x, y+dy, c)
			}
		}
		return img
	}
}
func maxIcon(c color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		m, t := sz/4, imax(2, sz/8)
		for x := m; x < sz-m; x++ {
			for dy := 0; dy < t; dy++ {
				img.SetRGBA(x, m+dy, c)
				img.SetRGBA(x, sz-m-1-dy, c)
			}
		}
		for y := m; y < sz-m; y++ {
			for dx := 0; dx < t; dx++ {
				img.SetRGBA(m+dx, y, c)
				img.SetRGBA(sz-m-1-dx, y, c)
			}
		}
		return img
	}
}
func handleIcon(fg, bg color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		m := sz / 4
		for y := m; y < sz-m; y++ {
			for x := m; x < sz-m; x++ {
				if x == m || x == sz-m-1 || y == m || y == sz-m-1 {
					img.SetRGBA(x, y, fg)
				} else {
					img.SetRGBA(x, y, bg)
				}
			}
		}
		return img
	}
}
func checkboxIcon(checked bool) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{240, 240, 240, 255}}, image.Point{}, draw.Src)
		b := color.RGBA{120, 120, 120, 255}
		for x := 0; x < sz; x++ {
			img.SetRGBA(x, 0, b)
			img.SetRGBA(x, sz-1, b)
		}
		for y := 0; y < sz; y++ {
			img.SetRGBA(0, y, b)
			img.SetRGBA(sz-1, y, b)
		}
		if checked {
			fg := color.RGBA{50, 150, 50, 255}
			for i := 0; i < sz/3; i++ {
				for t := -1; t <= 1; t++ {
					x1, y1 := sz/3-i, sz*2/3-i+t
					x2, y2 := sz/3+i*2, sz*2/3-i*2+t
					if x1 >= 0 && y1 >= 0 && x1 < sz && y1 < sz {
						img.SetRGBA(x1, y1, fg)
					}
					if x2 >= 0 && y2 >= 0 && x2 < sz && y2 < sz {
						img.SetRGBA(x2, y2, fg)
					}
				}
			}
		}
		return img
	}
}
func triRightIcon(c color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		for y := sz / 4; y < sz*3/4; y++ {
			dy := y - sz/2
			if dy < 0 {
				dy = -dy
			}
			for x := sz / 3; x < sz/3+(sz/4-dy) && x < sz; x++ {
				img.SetRGBA(x, y, c)
			}
		}
		return img
	}
}
func triDownIcon(c color.RGBA) func(int) *image.RGBA {
	return func(sz int) *image.RGBA {
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		for x := sz / 4; x < sz*3/4; x++ {
			dx := x - sz/2
			if dx < 0 {
				dx = -dx
			}
			for y := sz / 3; y < sz/3+(sz/4-dx) && y < sz; y++ {
				img.SetRGBA(x, y, c)
			}
		}
		return img
	}
}
func themeImg(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), &image.Uniform{c}, image.Point{}, draw.Src)
	b := color.RGBA{uint8(imax(0, int(c.R)-30)), uint8(imax(0, int(c.G)-30)), uint8(imax(0, int(c.B)-30)), 255}
	for x := 0; x < w; x++ {
		img.SetRGBA(x, 0, b)
		img.SetRGBA(x, h-1, b)
	}
	for y := 0; y < h; y++ {
		img.SetRGBA(0, y, b)
		img.SetRGBA(w-1, y, b)
	}
	return img
}
