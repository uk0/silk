package glui

import (
	"os"
	"runtime"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// systemFontCandidates returns a platform-ordered list of common CJK font
// paths to probe when constructing a Font. The first path that loads
// successfully wins. The list is deliberately conservative: every entry
// is shipped with the OS by default in a recent stable release, so we
// avoid pulling in fonts that the user may have explicitly removed.
//
// macOS: PingFang covers Simplified Chinese plus a substantial Traditional
// and Japanese subset. Hiragino Sans GB is the legacy fallback. STHeiti is
// included on every macOS install.
//
// Linux: Noto Sans CJK is the de-facto standard; the .ttc file is shipped
// by both Debian (fonts-noto-cjk apt) and Arch (noto-fonts-cjk pacman). We
// also probe the WenQuanYi family as a safer Linux-flavoured fallback. As
// a last resort DejaVu Sans is checked — it has no Han glyphs but extended
// Latin and Greek pages, useful for e.g. mathematical UIs.
//
// Windows: Microsoft YaHei (Simplified) is preinstalled on Windows 7+;
// SimSun is the Win XP-era fallback still bundled with current Windows.
//
// Returns nil on platforms we don't recognise.
func systemFontCandidates() []string {
	switch runtime.GOOS {
	case "darwin":
		// Apple has been moving Han-script fonts in and out of
		// /System/Library/Fonts/Supplemental/ across macOS releases (Songti
		// migrated in 10.15; Hiragino Sans GB lives at the canonical path
		// in current builds). Probe both locations so the discovery works
		// without depending on a specific macOS version.
		return []string{
			"/System/Library/Fonts/PingFang.ttc",
			"/System/Library/Fonts/Supplemental/PingFang.ttc",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/Supplemental/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/STHeiti Medium.ttc",
			"/System/Library/Fonts/STHeiti Light.ttc",
			"/System/Library/Fonts/Supplemental/Songti.ttc",
			"/Library/Fonts/Songti.ttc",
		}
	case "linux":
		return []string{
			"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/google-noto-cjk/NotoSansCJK-Regular.ttc",
			"/usr/share/fonts/wenquanyi/wqy-microhei/wqy-microhei.ttc",
			"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
			"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		}
	case "windows":
		return []string{
			`C:\Windows\Fonts\msyh.ttc`,
			`C:\Windows\Fonts\msyh.ttf`,
			`C:\Windows\Fonts\simsun.ttc`,
			`C:\Windows\Fonts\simhei.ttf`,
		}
	}
	return nil
}

// discoverSystemCJKFaces walks the platform candidate list and returns a
// face for the first successfully-loaded font, sized at the same point
// size as the primary. Returns nil if none of the candidates exist on the
// current host.
//
// The function is best-effort: the caller treats a nil/empty return as
// "no fallback" and proceeds with primary-only rendering. CJK characters
// will then surface as missing glyphs (zero-advance records) — visible
// gaps in the rendered string but no crash.
//
// We only return one face today. Most CJK fonts already cover Simplified
// + Traditional + Japanese + Korean from a single .ttc, so a single
// fallback typically suffices. The function returns a slice anyway so a
// future iteration can chain multiple discovered fonts (e.g. dedicated
// emoji or symbol fonts) without changing the call site.
func discoverSystemCJKFaces(size float64) []font.Face {
	face, _ := discoverSystemCJKFaceWithPath(size)
	if face == nil {
		return nil
	}
	return []font.Face{face}
}

// discoverSystemCJKFaceWithPath returns the first successfully-loaded
// CJK fallback face along with the filesystem path it came from. Tests
// use the path so a macOS upgrade that relocates PingFang surfaces in
// the CI log as "loaded /System/Library/Fonts/Supplemental/..." instead
// of "loaded /System/Library/Fonts/...". Returns ("", nil) when no
// candidate loads.
func discoverSystemCJKFaceWithPath(size float64) (font.Face, string) {
	for _, path := range systemFontCandidates() {
		face, err := loadFontFile(path, size)
		if err != nil || face == nil {
			continue
		}
		return face, path
	}
	return nil, ""
}

// loadFontFile reads a TTF or TTC file from disk and returns an opentype
// face at the given point size. TTC (font collection) files contain
// multiple faces in a single archive; we deliberately pick face 0, which
// for the major Han-script collections is the regular weight.
//
// Returns (nil, nil) if the path doesn't exist — the caller should treat
// that as "try next candidate" rather than an error.
func loadFontFile(path string, size float64) (font.Face, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// File missing is the common case on hosts without that font;
		// upstream code expects a nil-error skip rather than a stop.
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	// Try single-font parse first. sfnt.Parse rejects collection files,
	// so on .ttc inputs the second branch handles it.
	if ttf, err := sfnt.Parse(data); err == nil {
		return opentype.NewFace(ttf, &opentype.FaceOptions{
			Size:    size,
			DPI:     DefaultFontDPI,
			Hinting: font.HintingFull,
		})
	}

	col, err := sfnt.ParseCollection(data)
	if err != nil {
		return nil, err
	}
	if col.NumFonts() == 0 {
		return nil, nil
	}
	ttf, err := col.Font(0)
	if err != nil {
		return nil, err
	}
	return opentype.NewFace(ttf, &opentype.FaceOptions{
		Size:    size,
		DPI:     DefaultFontDPI,
		Hinting: font.HintingFull,
	})
}
