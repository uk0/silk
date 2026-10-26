package gui

import (
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

// CursorData owns its pixels; callers of LoadCursorData must close the Pixmap
// after constructing a native cursor. Hotspots use image-pixel coordinates.
type CursorData struct {
	paint.Pixmap
	HotX, HotY int
}

func validateCursorData(data CursorData) error {
	if data.Pixmap == nil {
		return errors.New("nil cursor pixmap")
	}
	w, h := data.Width(), data.Height()
	if w <= 0 || h <= 0 || w > 256 || h > 256 {
		return errors.New("cursor dimensions must be between 1 and 256 pixels")
	}
	if data.HotX < 0 || data.HotY < 0 || data.HotX >= w || data.HotY >= h {
		return errors.New("cursor hotspot is outside its image")
	}
	return nil
}

// LoadCursorData prefers an application PNG override (name[_hotX_hotY].png).
// Four built-in drag shapes remain available in SDK-only installations.
// Corrupt overrides are reported, not silently mistaken for absent resources.
func LoadCursorData(name string) (CursorData, error) {
	return loadCursorDataFromDir(filepath.Join(core.ResourceDir(), "cursor"), name)
}
func loadCursorDataFromDir(dir, name string) (CursorData, error) {
	name = strings.ToLower(name)
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return CursorData{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		base := strings.ToLower(entry.Name())
		if !strings.HasSuffix(base, ".png") {
			continue
		}
		base = strings.TrimSuffix(base, ".png")
		hotX, hotY := 0, 0
		if base != name {
			parts := strings.Split(strings.TrimPrefix(base, name+"_"), "_")
			if !strings.HasPrefix(base, name+"_") || len(parts) != 2 {
				continue
			}
			var ex, ey error
			hotX, ex = strconv.Atoi(parts[0])
			hotY, ey = strconv.Atoi(parts[1])
			if ex != nil || ey != nil {
				return CursorData{}, fmt.Errorf("invalid cursor hotspot: %s", entry.Name())
			}
		}
		path := filepath.Join(dir, entry.Name())
		f, err := os.Open(path)
		if err != nil {
			return CursorData{}, err
		}
		config, err := png.DecodeConfig(f)
		f.Close()
		if err != nil {
			return CursorData{}, err
		}
		if config.Width < 1 || config.Height < 1 || config.Width > 256 || config.Height > 256 {
			return CursorData{}, errors.New("cursor PNG exceeds 256 pixels")
		}
		if hotX < 0 || hotY < 0 || hotX >= config.Width || hotY >= config.Height {
			return CursorData{}, errors.New("cursor hotspot is outside its PNG")
		}
		pix, err := paint.LoadPngFile(path)
		return CursorData{pix, hotX, hotY}, err
	}
	if data, ok := builtinDropCursorData(name); ok {
		return data, nil
	}
	return CursorData{}, fmt.Errorf("cursor %q: %w", name, fs.ErrNotExist)
}

// Vector-built assets need neither optional resource folders nor raster fonts.
// The pointer tip and high-contrast silhouette are identical for all actions.
func builtinDropCursorData(name string) (CursorData, bool) {
	if name != "arrow" && name != "arrow-no" && name != "arrow-copy" && name != "arrow-link" {
		return CursorData{}, false
	}
	pix := paint.NewPixmap(32, 32)
	p := pix.NewPainter()
	p.MoveTo(2.5, 2.5)
	p.LineTo(2.5, 23)
	p.LineTo(8.5, 17.5)
	p.LineTo(13.5, 27)
	p.LineTo(18, 24.5)
	p.LineTo(13.5, 16)
	p.LineTo(22, 16)
	p.LineTo(2.5, 2.5)
	p.SetBrush1(paint.Color{R: 255, G: 255, B: 255, A: 255})
	p.FillPreserve()
	p.SetPen1(paint.Color{R: 25, G: 31, B: 42, A: 255}, 1.5)
	p.Stroke()
	if name != "arrow" {
		color := paint.Color{R: 0, G: 105, B: 215, A: 255}
		if name == "arrow-no" {
			color = paint.Color{R: 196, G: 36, B: 48, A: 255}
		}
		p.Arc(24, 24, 7, 0, 2*math.Pi)
		p.SetBrush1(color)
		p.FillPreserve()
		p.SetPen1(paint.Color{R: 255, G: 255, B: 255, A: 255}, 1.5)
		p.Stroke()
		p.SetPen1(paint.Color{R: 255, G: 255, B: 255, A: 255}, 2)
		switch name {
		case "arrow-no":
			p.Arc(24, 24, 4, 0, 2*math.Pi)
			p.Stroke()
			p.Line(21, 27, 27, 21)
			p.Stroke()
		case "arrow-copy":
			p.Line(20, 24, 28, 24)
			p.Line(24, 20, 24, 28)
			p.Stroke()
		case "arrow-link":
			p.MoveTo(20, 27)
			p.LineTo(27, 20)
			p.MoveTo(21, 20)
			p.LineTo(27, 20)
			p.LineTo(27, 26)
			p.Stroke()
		}
	}
	paint.ClosePainter(p)
	pix.Flush()
	return CursorData{pix, 2, 2}, true
}

// Compose bounded feedback once at drag start, never on mouse move. The
// preview is real content, not an empty allocation sized after that content.
func composeDropCursor(content paint.Pixmap, arrow CursorData) (CursorData, error) {
	if err := validateCursorData(arrow); err != nil {
		return CursorData{}, err
	}
	w, h := arrow.Width(), arrow.Height()
	cw, ch := 0.0, 0.0
	if content != nil && content.Width() > 0 && content.Height() > 0 {
		scale := min(1.0, 80/float64(content.Width()), 64/float64(content.Height()))
		cw, ch = float64(content.Width())*scale, float64(content.Height())*scale
		w, h = max(w, 16+int(math.Ceil(cw))), max(h, 16+int(math.Ceil(ch)))
	}
	pix := paint.NewPixmap(w, h)
	p := pix.NewPainter()
	if cw > 0 && ch > 0 {
		p.Save()
		p.Translate(16, 16)
		p.Scale(cw/float64(content.Width()), ch/float64(content.Height()))
		p.DrawPixmap(content)
		p.Restore()
	}
	p.DrawPixmap(arrow.Pixmap)
	paint.ClosePainter(p)
	pix.Flush()
	return CursorData{pix, arrow.HotX, arrow.HotY}, nil
}

var dndCursorData []CursorData // UI-thread cache, four small source images.
var dropCursorNames = [...]string{"arrow-no", "arrow", "arrow-copy", "arrow-link"}

// GenerateDropCursors returns ignore/move/copy/link cursors. UI thread only.
// Call ReleaseDropCursors after restoring the previous cursor; Window.DoDragDrop
// does this automatically. The caller retains ownership of content.
func GenerateDropCursors(content paint.Pixmap) []*Cursor {
	return generateDropCursors(content, NewCursorFromData)
}
func generateDropCursors(content paint.Pixmap, create func(CursorData) (*Cursor, error)) []*Cursor {
	if dndCursorData == nil {
		dndCursorData = make([]CursorData, len(dropCursorNames))
		for i, name := range dropCursorNames {
			data, err := LoadCursorData(name)
			if err != nil {
				core.Warn(err)
				data, _ = builtinDropCursorData(name)
			}
			dndCursorData[i] = data
		}
	}
	curs := make([]*Cursor, len(dropCursorNames))
	for i, arrow := range dndCursorData {
		data, err := composeDropCursor(content, arrow)
		if err == nil {
			curs[i], _ = create(data)
			paint.CloseSurface(data.Pixmap)
		}
		if curs[i] == nil {
			curs[i] = cursorArrow
			if i == 0 && cursorNo != nil {
				curs[i] = cursorNo
			}
		}
	}
	return curs
}

// ReleaseDropCursors releases temporary native handles deterministically.
// Shared cached/system fallbacks are never destroyed. UI thread only; do not
// release an installed cursor. Repeated calls and duplicate entries are safe.
func ReleaseDropCursors(cursors []*Cursor) {
	for i, cur := range cursors {
		if cur == nil {
			continue
		}
		shared := false
		for _, cached := range cursorCache {
			if cur == cached {
				shared = true
				break
			}
		}
		for j := 0; j < i && !shared; j++ {
			shared = cursors[j] == cur
		}
		if !shared {
			cur.destroy()
		}
	}
}
