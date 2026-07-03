// Cross-platform font loader for purecairo.
//
// Strategy mirrors libcairo's font-resolution: ask FontConfig (or the
// platform analogue) for a face, fall back to a known list, fall back
// to a built-in if nothing else works. Pure-Go has no FontConfig, so:
//
//   1. Probe a small set of well-known system font paths for the
//      requested family / weight / slant.
//   2. If the probe misses, fall through to the bundled Go-Regular
//      TrueType (golang.org/x/image/font/gofont/goregular) — that one
//      ships with the binary and works on every platform.
//   3. Latin glyphs come from the resolved face; CJK runes that the
//      face cannot rasterise route through a fallback face loaded
//      from the platform's CJK system font (PingFang on macOS, Noto on
//      Linux, MS YaHei on Windows).
//
// All faces are cached by `(family, size, bold, italic)` so widget
// repaints don't re-parse the font on every frame.

package purecairo

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// faceKey is the lookup key for the face cache.
type faceKey struct {
	family string
	size   int // pixel size, rounded
	bold   bool
	italic bool
}

var (
	faceCache   = make(map[faceKey]font.Face)
	faceCacheMu sync.Mutex
	cjkFallback font.Face
	cjkOnce     sync.Once
	systemFonts = make(map[string]string) // family-key → path
	sysOnce     sync.Once
	sfntCache   = make(map[string]*sfnt.Font) // path → parsed font
	sfntCacheMu sync.Mutex
)

// loadFace returns a font.Face for (family, size, bold, italic). Never
// returns nil — falls back to the bundled Go font if nothing else works.
func loadFace(family string, size int, bold, italic bool) font.Face {
	if size <= 0 {
		size = 12
	}
	key := faceKey{family: strings.ToLower(family), size: size, bold: bold, italic: italic}

	faceCacheMu.Lock()
	if f, ok := faceCache[key]; ok {
		faceCacheMu.Unlock()
		return f
	}
	faceCacheMu.Unlock()

	face := resolveFace(family, size, bold, italic)
	if face == nil {
		face = bundledFace(size, bold, italic)
	}

	faceCacheMu.Lock()
	faceCache[key] = face
	faceCacheMu.Unlock()
	return face
}

// resolveFace tries to find a system font matching family/weight/slant
// and rasterise it at `size` pixels. Returns nil on miss; the caller
// falls back to the bundled face.
func resolveFace(family string, size int, bold, italic bool) font.Face {
	sysOnce.Do(scanSystemFonts)

	candidates := systemPathsFor(family, bold, italic)
	for _, path := range candidates {
		if face := openSystemFont(path, size); face != nil {
			return face
		}
	}
	return nil
}

// scanSystemFonts walks the platform's font directories once and
// records every TTF / OTF / TTC by base filename (lower-cased, sans
// extension). The map is read by systemPathsFor when the caller asks
// for a specific family.
func scanSystemFonts() {
	dirs := platformFontDirs()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			lower := strings.ToLower(name)
			if !(strings.HasSuffix(lower, ".ttf") ||
				strings.HasSuffix(lower, ".otf") ||
				strings.HasSuffix(lower, ".ttc")) {
				continue
			}
			base := strings.TrimSuffix(lower, filepath.Ext(lower))
			full := filepath.Join(dir, name)
			// First-seen wins so /System/Library/Fonts beats Supplemental.
			if _, ok := systemFonts[base]; !ok {
				systemFonts[base] = full
			}
		}
	}
}

// platformFontDirs returns the canonical font directories per OS.
// Linux entries are best-effort: distros vary.
func platformFontDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return []string{
			"/System/Library/Fonts",
			"/System/Library/Fonts/Supplemental",
			"/Library/Fonts",
			filepath.Join(home, "Library/Fonts"),
		}
	case "windows":
		return []string{
			os.Getenv("SystemRoot") + "\\Fonts",
			"C:\\Windows\\Fonts",
		}
	default: // linux + bsd
		home, _ := os.UserHomeDir()
		return []string{
			"/usr/share/fonts",
			"/usr/share/fonts/truetype",
			"/usr/share/fonts/truetype/dejavu",
			"/usr/share/fonts/truetype/noto",
			"/usr/share/fonts/TTF",
			"/usr/share/fonts/opentype",
			"/usr/local/share/fonts",
			filepath.Join(home, ".fonts"),
			filepath.Join(home, ".local/share/fonts"),
		}
	}
}

// systemPathsFor maps a (family, bold, italic) request to a list of
// candidate font paths, ordered by preference. Each candidate may or
// may not exist; openSystemFont skips missing files.
func systemPathsFor(family string, bold, italic bool) []string {
	out := make([]string, 0, 8)
	add := func(stems ...string) {
		for _, stem := range stems {
			if path, ok := systemFonts[strings.ToLower(stem)]; ok {
				out = append(out, path)
			}
		}
	}

	famLow := strings.ToLower(family)
	monoFamilies := map[string]bool{
		"monaco":           true,
		"menlo":            true,
		"consolas":         true,
		"courier":          true,
		"courier new":      true,
		"sf mono":          true,
		"sfnsmono":         true,
		"dejavu sans mono": true,
	}

	switch {
	case famLow == "" || famLow == "system" || famLow == "default" || famLow == "sans-serif" || famLow == "sans":
		switch runtime.GOOS {
		case "darwin":
			if bold && italic {
				add("HelveticaNeue", "Helvetica", "Arial Bold Italic", "Arial")
			} else if bold {
				add("HelveticaNeue", "Helvetica", "Arial Bold", "Arial")
			} else if italic {
				add("HelveticaNeue", "Helvetica", "Arial Italic", "Arial")
			} else {
				add("HelveticaNeue", "Helvetica", "Arial", "SFNS")
			}
		case "windows":
			if bold && italic {
				add("arialbi", "arial")
			} else if bold {
				add("arialbd", "arial")
			} else if italic {
				add("ariali", "arial")
			} else {
				add("segoeui", "arial", "tahoma")
			}
		default:
			add("DejaVuSans", "DejaVuSans-Bold", "NotoSans-Regular", "NotoSans")
		}
	case monoFamilies[famLow]:
		switch runtime.GOOS {
		case "darwin":
			add("Menlo", "Monaco", "Courier", "SFNSMono")
		case "windows":
			add("consola", "consolab", "cour")
		default:
			add("DejaVuSansMono", "NotoSansMono-Regular", "NotoMono-Regular")
		}
	default:
		// Family-specific lookup: try the family name directly + bold/italic suffixes.
		add(family,
			family+" Bold",
			family+"-Bold",
			family+" Italic",
			family+"-Italic",
			family+" Bold Italic")
		// Generic sans-fallback at the end.
		switch runtime.GOOS {
		case "darwin":
			add("HelveticaNeue", "Helvetica", "Arial")
		case "windows":
			add("segoeui", "arial")
		default:
			add("DejaVuSans", "NotoSans-Regular")
		}
	}

	return out
}

// openSystemFont parses a TTF/OTF/TTC file and returns a face at `size`.
// .ttc collections fall through to the first font in the collection —
// good enough for system Helvetica / SF / Menlo where we want the
// default member.
func openSystemFont(path string, size int) font.Face {
	sfntCacheMu.Lock()
	cached, ok := sfntCache[path]
	sfntCacheMu.Unlock()
	if ok && cached != nil {
		return makeFace(cached, size)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	var f *sfnt.Font
	if strings.HasSuffix(strings.ToLower(path), ".ttc") {
		coll, err := opentype.ParseCollection(data)
		if err != nil || coll.NumFonts() == 0 {
			return nil
		}
		f, err = coll.Font(0)
		if err != nil {
			return nil
		}
	} else {
		f, err = opentype.Parse(data)
		if err != nil {
			return nil
		}
	}

	sfntCacheMu.Lock()
	sfntCache[path] = f
	sfntCacheMu.Unlock()

	return makeFace(f, size)
}

func makeFace(f *sfnt.Font, size int) font.Face {
	if size <= 0 {
		size = 12
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return nil
	}
	return face
}

// bundledFace returns a face from the embedded gofont TTFs. Always
// succeeds — the byte slices ship with the binary.
func bundledFace(size int, bold, italic bool) font.Face {
	var ttf []byte
	switch {
	case bold && italic:
		ttf = gobolditalic.TTF
	case bold:
		ttf = gobold.TTF
	case italic:
		ttf = goitalic.TTF
	default:
		ttf = goregular.TTF
	}
	f, err := opentype.Parse(ttf)
	if err != nil {
		// Static asset; parse should never fail at runtime, but be defensive.
		return basicFontFallback()
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return basicFontFallback()
	}
	return face
}

// loadCJKFallback returns a face that can rasterise CJK glyphs. Lazily
// initialised — the font is large and we only pay the parse cost when
// the first CJK rune appears.
func loadCJKFallback(size int) font.Face {
	cjkOnce.Do(func() {
		sysOnce.Do(scanSystemFonts)
	})
	candidates := cjkSystemPaths()
	for _, path := range candidates {
		if face := openSystemFont(path, size); face != nil {
			return face
		}
	}
	// No CJK system font — return Go Mono as a stub. Won't render Chinese
	// glyphs but the caller code-flow stays valid.
	f, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return basicFontFallback()
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{
		Size:    float64(size),
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return basicFontFallback()
	}
	return face
}

// basicFontFallback returns the bundled bitmap font as a last resort.
// Reached only if opentype parsing of an embedded asset fails — should
// be unreachable in practice.
func basicFontFallback() font.Face {
	return basicfont.Face7x13
}

func cjkSystemPaths() []string {
	out := make([]string, 0, 6)
	add := func(stems ...string) {
		for _, stem := range stems {
			if path, ok := systemFonts[strings.ToLower(stem)]; ok {
				out = append(out, path)
			}
		}
	}
	switch runtime.GOOS {
	case "darwin":
		add("PingFang", "STHeiti Medium", "STHeiti Light", "Songti", "AppleSDGothicNeo", "Heiti")
	case "windows":
		add("msyh", "msyhbd", "simhei", "simsun")
	default:
		add("NotoSansCJK-Regular", "NotoSansCJKsc-Regular",
			"NotoSansCJK", "WenQuanYiMicroHei", "NotoSansSC")
	}
	return out
}
