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
		//
		// macOS splits CJK script coverage across several files:
		//   - Hiragino Sans GB / PingFang / STHeiti / Songti — Han glyphs
		//     (Simplified + Traditional Chinese); no kana, limited Korean.
		//   - AquaKana — hiragana + katakana (Japanese phonetics) only.
		//   - AppleSDGothicNeo — full Korean coverage incl. Hangul.
		// The full chain is returned by discoverSystemCJKFaces in priority
		// order so a Japanese / Korean run still renders kana / Hangul
		// even when the Han-script primary doesn't carry them.
		return []string{
			// Han-script primaries.
			"/System/Library/Fonts/PingFang.ttc",
			"/System/Library/Fonts/Supplemental/PingFang.ttc",
			"/System/Library/Fonts/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/Supplemental/Hiragino Sans GB.ttc",
			"/System/Library/Fonts/STHeiti Medium.ttc",
			"/System/Library/Fonts/STHeiti Light.ttc",
			"/System/Library/Fonts/Supplemental/Songti.ttc",
			"/Library/Fonts/Songti.ttc",
			// Japanese kana coverage — required when the user's run
			// includes hiragana or katakana that the Han primary
			// can't render.
			"/System/Library/Fonts/AquaKana.ttc",
			"/System/Library/Fonts/Supplemental/AquaKana.ttc",
			// Korean coverage — Hangul syllables and Jamo.
			"/System/Library/Fonts/AppleSDGothicNeo.ttc",
			"/System/Library/Fonts/Supplemental/AppleGothic.ttf",
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

// discoverSystemCJKFaces walks the platform candidate list and returns
// every face that loads successfully, in the order they appear in
// systemFontCandidates. Returns nil if no candidate loads on the
// current host.
//
// The full chain matters because macOS in particular splits CJK script
// coverage across several files: Han-script primaries don't carry kana,
// kana files don't carry Hangul, and so on. Glyph() walks the chain
// in this order until a face with the requested rune is found, so a
// Japanese run on a CJK-mixed host renders correctly without the host
// having to figure out which one font covers everything.
//
// The function is best-effort: a nil/empty return leaves the host with
// primary-only rendering. CJK runes then surface as missing-glyph
// records — visible gaps but no crash.
func discoverSystemCJKFaces(size float64) []font.Face {
	var faces []font.Face
	for _, path := range systemFontCandidates() {
		face, err := loadFontFile(path, size)
		if err != nil || face == nil {
			continue
		}
		faces = append(faces, face)
	}
	return faces
}

// discoverSystemCJKFaceWithPath returns the first successfully-loaded
// CJK fallback face along with the filesystem path it came from. Tests
// use the path so a macOS upgrade that relocates PingFang surfaces in
// the CI log as "loaded /System/Library/Fonts/Supplemental/..." instead
// of "loaded /System/Library/Fonts/...". Returns ("", nil) when no
// candidate loads.
//
// Distinct from discoverSystemCJKFaces: this returns only the first
// match (used for tests that need a single observable path). Production
// font construction goes through discoverSystemCJKFaces to gather the
// full chain.
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
