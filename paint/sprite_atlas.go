package paint

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	stddraw "image/draw"
	"image/png"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
)

// SpriteAtlas is an immutable image grid with bounded, lazy per-icon device-size
// caches. Like Painter and Icon, its rendering methods belong to the UI thread.
// The input image is copied: later caller mutations cannot alter cached pixels.
type SpriteAtlas struct {
	icons map[string]*atlasIcon
	names []string
}
type atlasIcon struct {
	source *image.RGBA
	rect   image.Rectangle
	cache  []*subIcon
}

const spriteCacheLimit = 8

func NewSpriteAtlas(src image.Image, columns, rows int, names []string) (*SpriteAtlas, error) {
	if src == nil || columns <= 0 || rows <= 0 || columns > 256 || rows > 256 || len(names) != columns*rows {
		return nil, fmt.Errorf("sprite atlas: invalid grid")
	}
	b := src.Bounds()
	if b.Dx() <= 0 || b.Dy() <= 0 || b.Dx() > 4096 || b.Dy() > 4096 || b.Dx() < columns || b.Dy() < rows || b.Dx()*rows != b.Dy()*columns {
		return nil, fmt.Errorf("sprite atlas: expected equal square cells in an image up to 4096px")
	}
	a := &SpriteAtlas{icons: make(map[string]*atlasIcon), names: append([]string(nil), names...)}
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("sprite atlas: empty name")
		}
		if _, ok := a.icons[name]; ok {
			return nil, fmt.Errorf("sprite atlas: duplicate name %q", name)
		}
		a.icons[name] = nil
	}
	pixels := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	stddraw.Draw(pixels, pixels.Bounds(), src, b.Min, stddraw.Src)
	// Rational cell boundaries preserve every pixel when image generation crops
	// a square atlas to e.g. 1254px. Physical cells may differ by one pixel.
	for n, name := range names {
		col, row := n%columns, n/columns
		rect := image.Rect(col*b.Dx()/columns, row*b.Dy()/rows, (col+1)*b.Dx()/columns, (row+1)*b.Dy()/rows)
		a.icons[name] = &atlasIcon{source: pixels, rect: rect}
	}
	return a, nil
}
func (a *SpriteAtlas) Names() []string { return append([]string(nil), a.names...) }
func (a *SpriteAtlas) Icon(name string) (Icon, bool) {
	i, ok := a.icons[name]
	if !ok {
		return nil, false
	}
	return i, true
}
func (i *atlasIcon) IsAir() bool          { return false }
func (i *atlasIcon) AvailableSize() []int { return []int{i.rect.Dx()} }

// Pixmap returns a caller-owned image. The painter's hot path instead uses the
// cached sub-icon; it never decodes PNG or resamples on a warm repaint.
func (i *atlasIcon) Pixmap(size int) Pixmap {
	if size <= 0 || size > 4096 {
		return nil
	}
	p := NewPixmap(size, size)
	_ = p.SetImage(i.raster(size))
	return p
}
func (i *atlasIcon) raster(size int) *image.RGBA {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	// Scale premultiplied RGBA, never straight-alpha RGB. Sampling is restricted
	// to the cell so adjacent sprites cannot bleed at fractional scale.
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), i.source, i.rect, xdraw.Src, nil)
	return dst
}
func (i *atlasIcon) getSub(size int) *subIcon {
	if size <= 0 {
		return nil
	}
	if size > i.rect.Dx() {
		size = i.rect.Dx()
	}
	for n, sub := range i.cache {
		if sub.size == size {
			copy(i.cache[1:n+1], i.cache[:n])
			i.cache[0] = sub
			return sub
		}
	}
	p := NewPixmap(size, size)
	_ = p.SetImage(i.raster(size))
	sub := &subIcon{size: size, img: p}
	i.cache = append([]*subIcon{sub}, i.cache...)
	if len(i.cache) > spriteCacheLimit {
		i.cache[spriteCacheLimit] = nil
		i.cache = i.cache[:spriteCacheLimit]
	}
	return sub
}

//go:embed assets/silk-satin-v1.png
var satinAtlasPNG []byte

//go:embed assets/silk-satin-v1.regions.json
var satinRegionsJSON []byte
var satinOnce sync.Once
var satinAtlas *SpriteAtlas
var satinError error
var preferSatinIcons bool
var satinNames = []string{"new", "open", "save", "undo", "redo", "play", "build", "debug", "stop", "preview", "settings", "print", "gantt", "gauge", "valve", "tank"}
var satinAliases = map[string]string{"document-new": "new", "folder-open": "open", "folder": "open", "save": "save", "edit-undo": "undo", "edit-redo": "redo", "continue": "play", "play": "play", "run": "play", "build": "build", "debug": "debug", "stop": "stop", "preview": "preview", "gear": "settings", "settings": "settings", "print": "print", "printer": "print", "gantt": "gantt"}

// SatinIcon loads an embedded, image-model-generated sprite by its stable name.
// It requires neither an external resource directory nor network access.
func SatinIcon(name string) (Icon, error) {
	satinOnce.Do(func() {
		var src image.Image
		src, satinError = png.Decode(bytes.NewReader(satinAtlasPNG))
		if satinError == nil {
			satinAtlas, satinError = NewSpriteAtlas(src, 4, 4, satinNames)
			if satinError == nil {
				// Generated sheets are not pixel-perfect grids. Use measured
				// object regions and equal transparent padding, not neighbouring
				// cells that can contain fragments of the next row's artwork.
				var regions [][4]int
				satinError = json.Unmarshal(satinRegionsJSON, &regions)
				if satinError == nil && len(regions) != len(satinNames) {
					satinError = fmt.Errorf("invalid satin regions")
				}
				if satinError == nil {
					for n, b := range regions {
						width, height := b[2]-b[0], b[3]-b[1]
						side := max(width, height) + 12
						x, y := b[0]-(side-width)/2, b[1]-(side-height)/2
						rect := image.Rect(x, y, x+side, y+side)
						if width <= 0 || height <= 0 || !rect.In(src.Bounds()) {
							satinError = fmt.Errorf("invalid satin region %d", n)
							break
						}
						satinAtlas.icons[satinNames[n]].rect = rect
					}
				}
			}
		}
	})
	if satinError != nil {
		return nil, satinError
	}
	i, ok := satinAtlas.Icon(name)
	if !ok {
		return nil, fmt.Errorf("unknown satin icon %q", name)
	}
	return i, nil
}

// SetPreferSatinIcons opts common IDE actions into the generated material set.
// Small structural glyphs keep theme-aware vectors. Call on the UI thread;
// existing loaded icon handles follow the preference on their next draw.
func SetPreferSatinIcons(prefer bool) { preferSatinIcons = prefer }
func satinOverride(name string) Icon {
	if preferSatinIcons {
		if n, ok := satinAliases[name]; ok {
			i, _ := SatinIcon(n)
			return i
		}
	}
	return nil
}
