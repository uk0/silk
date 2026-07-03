//go:build ignore

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
)

func main() {
	sizes := []int{16, 22, 32, 48}
	base := "icon"

	for _, sz := range sizes {
		dir := filepath.Join(base, fmt.Sprintf("%dx%d", sz, sz))
		os.MkdirAll(dir, 0755)

		icons := map[string]func(int) *image.RGBA{
			"minimize":           drawMinimize,
			"maximize":           drawMaximize,
			"close-btn":          drawCloseBtn,
			"checkbox-checked":   drawCheckboxChecked,
			"checkbox-unchecked": drawCheckboxUnchecked,
			"expander-collapsed": drawExpanderCollapsed,
			"expander-expanded":  drawExpanderExpanded,
			"document":           drawDocument,
			"folder":             drawFolder,
			"demo":               drawDemo,
			"exit":               drawExit,
			"pencil":             drawPencil,
			"globe":              drawGlobe,
			"clipboard":          drawClipboard,
			"form":               drawForm,
			"window":             drawWindow,
			"error":              drawError,
			"apple":              drawApple,
			"handle-0-normal":    drawHandle0,
			"handle-1-normal":    drawHandle1,
			"handle-2-normal":    drawHandle2,
			"handle-0-active":    drawHandle0Active,
			"handle-1-active":    drawHandle1Active,
			"handle-2-active":    drawHandle2Active,
			"image-missing":      drawMissing,
			"radio-checked":      drawRadioChecked,
			"radio-unchecked":    drawRadioUnchecked,
			"combobox-arrow":     drawComboArrow,
			"spinbox-up":         drawSpinUp,
			"spinbox-down":       drawSpinDown,
			"design":             drawDesign,
			"save":               drawSave,
			"close":              drawClose,
			"preview":            drawPreview,
			"edit-undo":          drawEditUndo,
			"edit-redo":          drawEditRedo,
			"edit":               drawEdit,
			"propsheet":          drawPropsheet,
			"rect-tool":          drawRectTool,
			"arrow-tool":         drawArrowTool,
			"tree-view":          drawTreeView,
			"question":           drawQuestion,
			"align-left":         drawAlignLeft,
			"align-center":       drawAlignCenter,
			"align-right":        drawAlignRight,
			"run":                drawRun,
		}

		for name, gen := range icons {
			img := gen(sz)
			path := filepath.Join(dir, name+".png")
			saveImage(path, img)
		}
	}
	fmt.Println("Icons generated in icon/")
}

func saveImage(path string, img *image.RGBA) {
	f, err := os.Create(path)
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	defer f.Close()
	png.Encode(f, img)
}

func newImg(sz int) *image.RGBA {
	return image.NewRGBA(image.Rect(0, 0, sz, sz))
}

func setPixel(img *image.RGBA, x, y int, c color.RGBA) {
	if x >= 0 && y >= 0 && x < img.Bounds().Dx() && y < img.Bounds().Dy() {
		img.SetRGBA(x, y, c)
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, thick int) {
	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		for t := -thick / 2; t <= thick/2; t++ {
			setPixel(img, x0+t, y0, c)
			setPixel(img, x0, y0+t, c)
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func fillRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			setPixel(img, x+dx, y+dy, c)
		}
	}
}

func drawRect(img *image.RGBA, x, y, w, h int, c color.RGBA) {
	for dx := 0; dx < w; dx++ {
		setPixel(img, x+dx, y, c)
		setPixel(img, x+dx, y+h-1, c)
	}
	for dy := 0; dy < h; dy++ {
		setPixel(img, x, y+dy, c)
		setPixel(img, x+w-1, y+dy, c)
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				setPixel(img, cx+x, cy+y, c)
			}
		}
	}
}

func drawCircle(img *image.RGBA, cx, cy, r int, c color.RGBA) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			d := x*x + y*y
			if d >= (r-1)*(r-1) && d <= r*r {
				setPixel(img, cx+x, cy+y, c)
			}
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

var (
	dark  = color.RGBA{80, 80, 80, 255}
	mid   = color.RGBA{140, 140, 140, 255}
	light = color.RGBA{200, 200, 200, 255}
	white = color.RGBA{255, 255, 255, 255}
	blue  = color.RGBA{66, 133, 244, 255}
	red   = color.RGBA{220, 60, 60, 255}
	green = color.RGBA{60, 180, 80, 255}
	amber = color.RGBA{240, 180, 40, 255}
)

// ─── Icon Generators ───

func drawMinimize(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 4
	y := sz / 2
	drawLine(img, m, y, sz-m, y, dark, 1)
	return img
}

func drawMaximize(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 4
	drawRect(img, m, m, sz-2*m, sz-2*m, dark)
	fillRect(img, m, m, sz-2*m, 2, dark)
	return img
}

func drawCloseBtn(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 4
	drawLine(img, m, m, sz-m-1, sz-m-1, dark, 1)
	drawLine(img, sz-m-1, m, m, sz-m-1, dark, 1)
	return img
}

func drawCheckboxChecked(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 6
	// box
	fillRect(img, m, m, sz-2*m, sz-2*m, blue)
	drawRect(img, m, m, sz-2*m, sz-2*m, color.RGBA{40, 100, 200, 255})
	// checkmark
	cx, cy := sz/2, sz/2
	drawLine(img, cx-sz/5, cy, cx-sz/10, cy+sz/5, white, 1)
	drawLine(img, cx-sz/10, cy+sz/5, cx+sz/4, cy-sz/5, white, 1)
	return img
}

func drawCheckboxUnchecked(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 6
	fillRect(img, m, m, sz-2*m, sz-2*m, white)
	drawRect(img, m, m, sz-2*m, sz-2*m, mid)
	return img
}

func drawExpanderCollapsed(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	s := sz / 4
	// right-pointing triangle
	for dy := -s; dy <= s; dy++ {
		w := s - abs(dy)
		for dx := 0; dx < w; dx++ {
			setPixel(img, cx-s/2+dx, cy+dy, dark)
		}
	}
	return img
}

func drawExpanderExpanded(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	s := sz / 4
	// down-pointing triangle
	for dx := -s; dx <= s; dx++ {
		h := s - abs(dx)
		for dy := 0; dy < h; dy++ {
			setPixel(img, cx+dx, cy-s/2+dy, dark)
		}
	}
	return img
}

func drawDocument(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	fold := sz / 4
	// page body
	fillRect(img, m, m, sz-2*m, sz-2*m, white)
	drawRect(img, m, m, sz-2*m, sz-2*m, mid)
	// fold corner
	drawLine(img, sz-m-fold, m, sz-m, m+fold, mid, 1)
	drawLine(img, sz-m-fold, m, sz-m-fold, m+fold, mid, 1)
	drawLine(img, sz-m-fold, m+fold, sz-m, m+fold, mid, 1)
	// text lines
	lx := m + 3
	for i := 0; i < 3; i++ {
		ly := m + fold + 3 + i*3
		if ly < sz-m-2 {
			w := sz - 2*m - 6 - (i%2)*4
			drawLine(img, lx, ly, lx+w, ly, light, 0)
		}
	}
	return img
}

func drawFolder(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	tabW := sz / 3
	tabH := sz / 6
	// tab
	fillRect(img, m, m+tabH, tabW, tabH, amber)
	// body
	fillRect(img, m, m+tabH*2, sz-2*m, sz-2*m-tabH*2, amber)
	drawRect(img, m, m+tabH*2, sz-2*m, sz-2*m-tabH*2, color.RGBA{200, 150, 20, 255})
	return img
}

func drawDemo(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	fillCircle(img, cx, cy, r, blue)
	fillCircle(img, cx, cy, r/2, white)
	return img
}

func drawExit(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 4
	// door
	fillRect(img, m, m, sz/2, sz-2*m, light)
	drawRect(img, m, m, sz/2, sz-2*m, mid)
	// arrow
	ax := sz/2 + sz/6
	ay := sz / 2
	drawLine(img, ax, ay, sz-m, ay, red, 1)
	drawLine(img, sz-m-sz/8, ay-sz/8, sz-m, ay, red, 1)
	drawLine(img, sz-m-sz/8, ay+sz/8, sz-m, ay, red, 1)
	return img
}

func drawPencil(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 4
	drawLine(img, m, sz-m, sz-m, m, amber, 1)
	drawLine(img, m+1, sz-m, sz-m+1, m, amber, 1)
	setPixel(img, m, sz-m+1, color.RGBA{60, 60, 60, 255})
	return img
}

func drawGlobe(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	drawCircle(img, cx, cy, r, blue)
	// horizontal lines
	drawLine(img, cx-r, cy, cx+r, cy, blue, 0)
	drawLine(img, cx-r+2, cy-r/2, cx+r-2, cy-r/2, blue, 0)
	drawLine(img, cx-r+2, cy+r/2, cx+r-2, cy+r/2, blue, 0)
	// vertical ellipse
	for a := 0.0; a < 2*math.Pi; a += 0.05 {
		x := cx + int(float64(r)/3*math.Cos(a))
		y := cy + int(float64(r)*math.Sin(a))
		setPixel(img, x, y, blue)
	}
	return img
}

func drawClipboard(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	// board
	fillRect(img, m+1, m+2, sz-2*m-2, sz-2*m-2, white)
	drawRect(img, m+1, m+2, sz-2*m-2, sz-2*m-2, mid)
	// clip
	cw := sz / 4
	cx := sz/2 - cw/2
	fillRect(img, cx, m-1, cw, 4, dark)
	return img
}

func drawForm(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	fillRect(img, m, m, sz-2*m, sz-2*m, white)
	drawRect(img, m, m, sz-2*m, sz-2*m, mid)
	// title bar
	fillRect(img, m, m, sz-2*m, 3, blue)
	return img
}

func drawWindow(sz int) *image.RGBA {
	return drawForm(sz)
}

func drawError(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	fillCircle(img, cx, cy, r, red)
	drawLine(img, cx-r/2, cy-r/2, cx+r/2, cy+r/2, white, 1)
	drawLine(img, cx+r/2, cy-r/2, cx-r/2, cy+r/2, white, 1)
	return img
}

func drawApple(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2+sz/8
	r := sz / 3
	fillCircle(img, cx, cy, r, green)
	// stem
	drawLine(img, cx, cy-r, cx+2, cy-r-3, color.RGBA{100, 60, 20, 255}, 0)
	return img
}

func drawHandleBase(sz int, c color.RGBA) *image.RGBA {
	img := newImg(sz)
	r := sz / 3
	if r < 2 {
		r = 2
	}
	cx, cy := sz/2, sz/2
	fillCircle(img, cx, cy, r, c)
	return img
}

func drawHandle0(sz int) *image.RGBA       { return drawHandleBase(sz, light) }
func drawHandle1(sz int) *image.RGBA       { return drawHandleBase(sz, light) }
func drawHandle2(sz int) *image.RGBA       { return drawHandleBase(sz, light) }
func drawHandle0Active(sz int) *image.RGBA { return drawHandleBase(sz, blue) }
func drawHandle1Active(sz int) *image.RGBA { return drawHandleBase(sz, blue) }
func drawHandle2Active(sz int) *image.RGBA { return drawHandleBase(sz, blue) }

func drawMissing(sz int) *image.RGBA {
	img := newImg(sz)
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{255, 200, 200, 255}}, image.Point{}, draw.Src)
	drawLine(img, 0, 0, sz-1, sz-1, red, 1)
	drawLine(img, sz-1, 0, 0, sz-1, red, 1)
	return img
}

func drawRadioChecked(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	drawCircle(img, cx, cy, r, mid)
	fillCircle(img, cx, cy, r/2, blue)
	return img
}

func drawRadioUnchecked(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	drawCircle(img, cx, cy, r, mid)
	return img
}

func drawComboArrow(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	s := sz / 5
	drawLine(img, cx-s, cy-s/2, cx, cy+s/2, dark, 1)
	drawLine(img, cx, cy+s/2, cx+s, cy-s/2, dark, 1)
	return img
}

func drawSpinUp(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	s := sz / 5
	drawLine(img, cx-s, cy+s/2, cx, cy-s/2, dark, 1)
	drawLine(img, cx, cy-s/2, cx+s, cy+s/2, dark, 1)
	return img
}

func drawSpinDown(sz int) *image.RGBA {
	return drawComboArrow(sz)
}

// ─── New Icon Generators ───

// design — pencil on a form/canvas
func drawDesign(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	// canvas background
	fillRect(img, m, m, sz-2*m, sz-2*m, white)
	drawRect(img, m, m, sz-2*m, sz-2*m, mid)
	// pencil diagonal across the canvas
	drawLine(img, m+2, sz-m-2, sz-m-2, m+2, amber, 1)
	drawLine(img, m+3, sz-m-2, sz-m-1, m+2, amber, 1)
	// pencil tip
	setPixel(img, m+1, sz-m-1, dark)
	return img
}

// save — floppy disk icon
func drawSave(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	w := sz - 2*m
	h := sz - 2*m
	// disk body
	fillRect(img, m, m, w, h, blue)
	drawRect(img, m, m, w, h, color.RGBA{40, 100, 200, 255})
	// metal slider area (top)
	slw := w * 2 / 3
	slx := m + (w-slw)/2
	fillRect(img, slx, m, slw, h/3, mid)
	// small notch in slider
	nw := slw / 3
	nx := slx + slw - nw - 1
	fillRect(img, nx, m, nw, h/3, dark)
	// label area (bottom)
	lm := 2
	fillRect(img, m+lm, m+h*2/3, w-2*lm, h/3-1, white)
	return img
}

// close — X mark in circle
func drawClose(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	drawCircle(img, cx, cy, r, red)
	s := r * 2 / 3
	drawLine(img, cx-s, cy-s, cx+s, cy+s, red, 1)
	drawLine(img, cx+s, cy-s, cx-s, cy+s, red, 1)
	return img
}

// preview — eye icon (oval with inner circle)
func drawPreview(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	rx := sz / 3 // horizontal radius
	ry := sz / 5 // vertical radius
	// draw eye outline as ellipse
	for a := 0.0; a < 2*math.Pi; a += 0.03 {
		x := cx + int(float64(rx)*math.Cos(a))
		y := cy + int(float64(ry)*math.Sin(a))
		setPixel(img, x, y, dark)
	}
	// top and bottom pointed edges
	drawLine(img, cx-rx, cy, cx-rx+2, cy-1, dark, 0)
	drawLine(img, cx-rx, cy, cx-rx+2, cy+1, dark, 0)
	drawLine(img, cx+rx, cy, cx+rx-2, cy-1, dark, 0)
	drawLine(img, cx+rx, cy, cx+rx-2, cy+1, dark, 0)
	// iris
	ir := sz / 7
	if ir < 1 {
		ir = 1
	}
	fillCircle(img, cx, cy, ir, blue)
	// pupil
	pr := ir / 2
	if pr < 1 {
		pr = 1
	}
	fillCircle(img, cx, cy, pr, dark)
	return img
}

// edit-undo — curved arrow pointing left
func drawEditUndo(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	// draw arc (top half going left)
	for a := -0.3; a < math.Pi+0.3; a += 0.04 {
		x := cx + int(float64(r)*math.Cos(a))
		y := cy - int(float64(r)*math.Sin(a))
		setPixel(img, x, y, dark)
	}
	// arrowhead pointing left at the left end of the arc
	ax := cx - r
	ay := cy
	drawLine(img, ax, ay, ax+sz/6, ay-sz/6, dark, 1)
	drawLine(img, ax, ay, ax+sz/6, ay+sz/6, dark, 1)
	return img
}

// edit-redo — curved arrow pointing right
func drawEditRedo(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	// draw arc (top half going right)
	for a := -0.3; a < math.Pi+0.3; a += 0.04 {
		x := cx - int(float64(r)*math.Cos(a))
		y := cy - int(float64(r)*math.Sin(a))
		setPixel(img, x, y, dark)
	}
	// arrowhead pointing right at the right end of the arc
	ax := cx + r
	ay := cy
	drawLine(img, ax, ay, ax-sz/6, ay-sz/6, dark, 1)
	drawLine(img, ax, ay, ax-sz/6, ay+sz/6, dark, 1)
	return img
}

// edit — pencil/pen icon (thicker than the existing pencil)
func drawEdit(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	// pencil body (diagonal)
	drawLine(img, m, sz-m, sz-m, m, amber, 1)
	drawLine(img, m+1, sz-m, sz-m+1, m, amber, 1)
	drawLine(img, m-1, sz-m, sz-m-1, m, amber, 1)
	// tip
	setPixel(img, m-1, sz-m+1, dark)
	setPixel(img, m, sz-m+1, dark)
	// eraser end
	drawLine(img, sz-m-1, m, sz-m+1, m-1, color.RGBA{220, 130, 130, 255}, 1)
	return img
}

// propsheet — list with checkboxes
func drawPropsheet(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	rows := 3
	rh := (sz - 2*m) / rows
	for i := 0; i < rows; i++ {
		y := m + i*rh
		// small checkbox
		bsz := rh - 2
		if bsz < 2 {
			bsz = 2
		}
		fillRect(img, m, y, bsz, bsz, white)
		drawRect(img, m, y, bsz, bsz, mid)
		// checkmark in first two rows
		if i < 2 {
			drawLine(img, m+1, y+bsz/2, m+bsz/2, y+bsz-1, blue, 0)
			drawLine(img, m+bsz/2, y+bsz-1, m+bsz-1, y+1, blue, 0)
		}
		// text line
		lx := m + bsz + 2
		ly := y + bsz/2
		lineW := sz - m - lx - 1
		if lineW > 0 {
			drawLine(img, lx, ly, lx+lineW, ly, dark, 0)
		}
	}
	return img
}

// rect-tool — dashed rectangle
func drawRectTool(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 4
	w := sz - 2*m
	h := sz - 2*m
	dash := 2
	// top and bottom edges (dashed)
	for dx := 0; dx < w; dx++ {
		if (dx/dash)%2 == 0 {
			setPixel(img, m+dx, m, blue)
			setPixel(img, m+dx, m+h-1, blue)
		}
	}
	// left and right edges (dashed)
	for dy := 0; dy < h; dy++ {
		if (dy/dash)%2 == 0 {
			setPixel(img, m, m+dy, blue)
			setPixel(img, m+w-1, m+dy, blue)
		}
	}
	return img
}

// arrow-tool — cursor arrow
func drawArrowTool(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	// arrow body: top-left to bottom, triangular
	tip := m
	bot := sz - m
	right := sz/2 + 1
	// fill arrow triangle
	h := bot - tip
	for dy := 0; dy < h; dy++ {
		w := (dy * (right - tip)) / h
		for dx := 0; dx <= w; dx++ {
			setPixel(img, tip+dx, tip+dy, dark)
		}
	}
	// outline left edge
	drawLine(img, tip, tip, tip, bot, dark, 0)
	// outline right diagonal
	drawLine(img, tip, tip, right, bot-sz/5, dark, 0)
	// bottom edge
	drawLine(img, tip, bot, right, bot-sz/5, dark, 0)
	return img
}

// tree-view — tree hierarchy icon
func drawTreeView(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	ns := sz / 6 // node size
	if ns < 2 {
		ns = 2
	}
	// root node
	rx, ry := m+ns/2, m+ns/2
	fillRect(img, m, m, ns, ns, dark)
	// vertical line down from root
	drawLine(img, rx, ry+ns/2, rx, sz-m-ns/2, mid, 0)
	// child 1 (upper)
	c1x, c1y := m+sz/3, m+sz/4
	fillRect(img, c1x, c1y, ns, ns, blue)
	drawLine(img, rx, c1y+ns/2, c1x, c1y+ns/2, mid, 0)
	// child 2 (lower)
	c2x, c2y := m+sz/3, sz-m-ns
	fillRect(img, c2x, c2y, ns, ns, blue)
	drawLine(img, rx, c2y+ns/2, c2x, c2y+ns/2, mid, 0)
	// grandchild from child1
	gcx, gcy := m+sz*2/3, c1y+ns+2
	if gcx+ns < sz && gcy+ns < sz {
		fillRect(img, gcx, gcy, ns, ns, green)
		drawLine(img, c1x+ns/2, c1y+ns, c1x+ns/2, gcy+ns/2, mid, 0)
		drawLine(img, c1x+ns/2, gcy+ns/2, gcx, gcy+ns/2, mid, 0)
	}
	return img
}

// question — question mark in circle
func drawQuestion(sz int) *image.RGBA {
	img := newImg(sz)
	cx, cy := sz/2, sz/2
	r := sz / 3
	// circle
	drawCircle(img, cx, cy, r, blue)
	// question mark: small arc at top
	qr := r / 2
	if qr < 2 {
		qr = 2
	}
	// upper curve of question mark
	for a := -0.5; a < math.Pi+0.2; a += 0.05 {
		x := cx + int(float64(qr)*math.Cos(a))
		y := cy - qr/2 - int(float64(qr)*math.Sin(a))
		setPixel(img, x, y, dark)
	}
	// stem going down from the curve
	drawLine(img, cx, cy-1, cx, cy+qr/2, dark, 0)
	// dot at the bottom
	setPixel(img, cx, cy+qr/2+2, dark)
	if sz >= 22 {
		setPixel(img, cx+1, cy+qr/2+2, dark)
		setPixel(img, cx, cy+qr/2+3, dark)
		setPixel(img, cx+1, cy+qr/2+3, dark)
	}
	return img
}

// align-left — three lines aligned to left edge
func drawAlignLeft(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	// Left alignment bar
	drawLine(img, m, m, m, sz-m, blue, 1)
	// Three horizontal lines of different lengths, left-aligned
	for i := 0; i < 3; i++ {
		y := m + 2 + i*(sz-2*m-4)/2
		w := sz - 2*m - i*sz/6
		drawLine(img, m+2, y, m+2+w, y, dark, 1)
	}
	return img
}

// align-center — three centered lines
func drawAlignCenter(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	cx := sz / 2
	// Center line
	drawLine(img, cx, m, cx, sz-m, blue, 0)
	// Three centered horizontal lines
	widths := []int{sz - 2*m, sz / 2, sz * 2 / 3}
	for i, w := range widths {
		y := m + 2 + i*(sz-2*m-4)/2
		drawLine(img, cx-w/2, y, cx+w/2, y, dark, 1)
	}
	return img
}

// align-right — three lines aligned to right edge
func drawAlignRight(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 5
	rx := sz - m
	// Right alignment bar
	drawLine(img, rx, m, rx, sz-m, blue, 1)
	// Three horizontal lines of different lengths, right-aligned
	for i := 0; i < 3; i++ {
		y := m + 2 + i*(sz-2*m-4)/2
		w := sz - 2*m - i*sz/6
		drawLine(img, rx-2-w, y, rx-2, y, dark, 1)
	}
	return img
}

// run — green play triangle
func drawRun(sz int) *image.RGBA {
	img := newImg(sz)
	m := sz / 4
	// Fill green triangle
	for dy := 0; dy < sz-2*m; dy++ {
		y := m + dy
		maxW := (sz - 2*m) * dy / (sz - 2*m)
		if dy > (sz-2*m)/2 {
			maxW = (sz - 2*m) * (sz - 2*m - dy) / (sz - 2*m)
		}
		for dx := 0; dx < maxW; dx++ {
			setPixel(img, m+dx, y, green)
		}
	}
	// Outline
	drawLine(img, m, m, sz-m, sz/2, green, 1)
	drawLine(img, sz-m, sz/2, m, sz-m, green, 1)
	drawLine(img, m, m, m, sz-m, green, 1)
	return img
}
