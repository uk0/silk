package paint

import (
	"silk/cairo"
	"silk/core"
	//	"silk/geom"
	//"silk/shell"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	iconRootDir  string
	iconCache    map[string]*cachedIcon
	iconSrcCache map[string]*iconSrc
	accessId     uint32
	errIcon      *icon
	airIcon      = new(icon)
)

type Icon interface {
	AvailableSize() []int
	IsAir() bool
	Pixmap(size int) Pixmap
}

type subIcon struct {
	size int
	path string
	img  *cairoSurface
	pat  *cairo.Pattern
}

type icon struct {
	name string
	subs []*subIcon
}

type cachedIcon struct {
	access uint32
	icon   *icon
}

type iconSrc struct {
	subs []*subIconSrc
}

type subIconSrc struct {
	size int
	path string
}

func (this *iconSrc) add(path, fn string, size int) {
	path = path[5:] + "/" + fn
	for i, v := range this.subs {
		if v.size == size {
			core.Warn(`conflict icon files: "` + v.path + `" and "` + path + `"`)
			v.path = path
			return
		}
		if v.size > size {
			this.subs = append(this.subs, nil)
			copy(this.subs[i+1:], this.subs[i:])
			this.subs[i] = &subIconSrc{size, path}
			return
		}
	}
	this.subs = append(this.subs, &subIconSrc{size, path})
}

func parse(fn string) (name string, size int, ok bool) {
	low := strings.ToLower(fn)
	ok = strings.HasSuffix(low, ".png")
	if !ok {
		return
	}
	k := low[:len(low)-4]

	// Use LastIndex to find the LAST underscore so that names with
	// hyphens or extra underscores (e.g. "close-btn_16") are parsed
	// correctly as name="close-btn", size=16.
	lastUnderscore := strings.LastIndex(k, "_")
	if lastUnderscore > 0 {
		sizeStr := k[lastUnderscore+1:]
		sz, err := strconv.Atoi(sizeStr)
		if err == nil && sz > 0 {
			name = k[:lastUnderscore]
			size = sz
			return
		}
	}
	// No valid size suffix found
	name = k
	size = 1
	return
}

// 加载图标
// 如果加载失败, 则返回"大红叉"图标, 名为"image-missing"
// 此函数将缓存已加载的图标, 以优化加载效率
// 更换图标后需要重启才能生效
func LoadIcon(name string) Icon {
	if iconSrcCache == nil {
		preloadPath()
		iconCache = make(map[string]*cachedIcon)
	}

	if name == "" {
		return airIcon
	}

	accessId++

	cached, ok := iconCache[name]
	if ok {
		cached.access = accessId
		return cached.icon
	}
	var ico *icon
	if src, ok := iconSrcCache[name]; ok {
		//im = src.load(size)
		ico = new(icon)
		for _, ss := range src.subs {
			ico.subs = append(ico.subs, &subIcon{ss.size, ss.path, nil, nil})
		}
		ico.name = name
	} else if name == "image-missing" {
		ico = genMissingIcon()
	} else if drawer, ok := proceduralFallbacks[name]; ok {
		// Procedural fallback for known UI affordances. Lets silkide
		// look right even when the resource theme hasn't been
		// installed (the red-X "image-missing" is reserved for truly
		// unknown names so the divergence is still loud).
		ico = genProceduralIcon(name, drawer)
	} else {
		core.Log(`icon not found: "` + name + `"`)
		ico = LoadIcon("image-missing").(*icon)
	}
	iconCache[name] = &cachedIcon{accessId, ico}
	return ico
}

func preload1(path string) {
	dir, err := os.Open(iconRootDir + path)
	if err != nil {
		core.Log(err)
		return
	}
	defer dir.Close()

	// Resource icons may live in size subdirectories ("16x16/foo.png")
	// rather than embedded in the filename ("foo_16.png"). Extract the
	// size from the directory name when it matches the NxN pattern;
	// the parse() filename suffix takes precedence so a file with an
	// explicit "_NN" suffix can still override the directory size.
	dirSize := dirSizeFromName(filepath.Base(path))

	infos, err := dir.Readdir(-1)
	for _, info := range infos {
		n := info.Name()
		if n[0] == '.' {
			continue
		}
		if info.IsDir() {
			preload1(path + "/" + n)
			continue
		}

		name, size, ok := parse(n)
		if !ok {
			continue
		}
		// "_NN" filename suffix returned size=1 means parse fell back
		// to the no-suffix path. Prefer the directory size in that
		// case so 16x16/edit-undo.png and 22x22/edit-undo.png stop
		// colliding at size=1 and overwriting each other.
		if size == 1 && dirSize > 0 {
			size = dirSize
		}
		src, ok := iconSrcCache[name]
		if !ok {
			src = new(iconSrc)
			iconSrcCache[name] = src
		}
		src.add(path, n, size)
	}
}

// dirSizeFromName extracts the side length from a "NxN" or "NxM"
// directory name like "16x16" or "22x22". Returns 0 when the name
// doesn't match — preload1's caller falls back to the filename
// parse path in that case.
func dirSizeFromName(dir string) int {
	x := strings.Index(dir, "x")
	if x <= 0 || x == len(dir)-1 {
		return 0
	}
	n, err := strconv.Atoi(dir[:x])
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func preloadPath() {
	iconSrcCache = make(map[string]*iconSrc)
	iconRootDir = core.ResourceDir()
	preload1("/icon")
	iconRootDir += "/icon"

}

func genMissingIcon() *icon {
	if errIcon == nil {
		errIcon = new(icon)
		for _, size := range []int{16, 22, 32, 48} {
			img := genMissingSubIcon(size)
			//pat := NewPatternForSurface(img)
			errIcon.subs = append(errIcon.subs, &subIcon{size, "", img, nil})
		}
		errIcon.name = "image-missing"
	}

	return errIcon
}

func genMissingSubIcon(size int) *cairoSurface {

	w := float64(size)
	s := NewPixmap(size, size)
	cc := s.NewContext()
	lw := 1 + w*0.05
	cc.Rectangle(lw, lw, w-lw*2, w-lw*2)
	cc.SetSourceRGBA(1, 1, 1, 0.5)
	cc.SetOperator(cairo.OPERATOR_SOURCE)
	cc.FillPreserve()
	cc.SetOperator(cairo.OPERATOR_OVER)

	cc.MoveTo(lw, lw)
	cc.LineTo(w-lw, w-lw)
	cc.MoveTo(w-lw, lw)
	cc.LineTo(lw, w-lw)
	//pen := NewPen(lw, 1, 0, 0, 1)
	//cc.SetPen(pen)
	cc.SetSourceRGB(1, 0, 0)
	cc.SetLineWidth(lw)
	//	pen := paint.NewPen(paint.Color{255, 0, 0, 255}, lw)
	cc.Stroke()

	return s
}

func (this *icon) AvailableSize() []int {
	var ret []int
	for _, sub := range this.subs {
		ret = append(ret, sub.size)
	}
	return ret
}

func (this *icon) IsAir() bool {
	return len(this.subs) == 0
}

func (this *icon) String() string {
	return this.name
}

func (this *icon) Pixmap(size int) Pixmap {

	sub := this.getNearest(size)
	if sub == nil {
		return nil
	}
	//	sz := sub.img.Width()

	pixmap := NewPixmap(size, size)
	g := pixmap.NewPainter()

	/*if sz != size {
		if sub.pat == nil {
			sub.pat = cairo.NewPatternForSurface(sub.img.Surface)
		}
		var m geom.Mat3x2
		scale := float64(sz) / float64(size)
		m.InitScale(scale, scale)
		sub.pat.SetMatrix(&m)
		g.SetSourcePattern(sub.pat)
	} else {
		g.SetSourceSurface(sub.img, 0, 0)
	}

	g.SetOperator(OpSource)
	g.Paint()
	*/
	g.DrawPixmap5(0, 0, float64(size), float64(size), sub.img)
	pixmap.Flush()
	return pixmap
}

func (this *subIcon) load() bool {
	if this.img == nil {
		path := iconRootDir + this.path
		img, err := LoadPngFile(path)
		if err == nil {
			this.img = img
			return true
		}
		return false
	}
	return true
}

func (this *icon) getNearest(size int) *subIcon {
	n := len(this.subs)
	if n == 0 {
		return nil
	}
	var i int
	var sub *subIcon
	for i, sub = range this.subs {
		if sub.size >= size {
			break
		}
	}

	if i == n {
		i = n - 1
	}

	sub = this.subs[i]
	if sub.load() {
		return sub
	}

	for j := i + 1; j < n; j++ {
		if this.subs[j].load() {
			return this.subs[j]
		}
	}

	for j := i - 1; j >= 0; j-- {
		if this.subs[j].load() {
			return this.subs[j]
		}
	}

	return nil
}

func AirIcon() Icon {
	return airIcon
}
