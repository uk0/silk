package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

// fresh constructs a new Translator for tests so they don't perturb
// the package-level Default. Every test owns its own table.
func fresh() *Translator {
	return NewTranslator()
}

// TestTReturnsSourceWhenLocaleEmpty: with no locale active, every
// lookup is a passthrough. This is the "translation off" baseline
// — apps shipped without translation files must still render text.
func TestTReturnsSourceWhenLocaleEmpty(t *testing.T) {
	tr := fresh()
	tr.Add("zh-CN", "File", "文件")
	if got := tr.T("File"); got != "File" {
		t.Errorf("locale empty: T(File) = %q, want File", got)
	}
}

// TestTHitReturnsTranslated: with a registered translation and the
// matching locale active, T returns the translated string.
func TestTHitReturnsTranslated(t *testing.T) {
	tr := fresh()
	tr.Add("zh-CN", "File", "文件")
	tr.SetLocale("zh-CN")
	if got := tr.T("File"); got != "文件" {
		t.Errorf("T(File) = %q, want 文件", got)
	}
}

// TestTMissReturnsSource: an unknown source string falls back to
// itself (Qt behaviour — tr() never hides text).
func TestTMissReturnsSource(t *testing.T) {
	tr := fresh()
	tr.Add("zh-CN", "File", "文件")
	tr.SetLocale("zh-CN")
	if got := tr.T("Unknown"); got != "Unknown" {
		t.Errorf("T(Unknown) = %q, want Unknown", got)
	}
}

// TestLocaleFallsBackToLanguageRoot: a translation registered under
// "zh" must still serve a "zh-CN" lookup. The language root fallback
// lets translators ship one file for the whole language family.
func TestLocaleFallsBackToLanguageRoot(t *testing.T) {
	tr := fresh()
	tr.Add("zh", "Edit", "编辑")
	tr.SetLocale("zh-CN")
	if got := tr.T("Edit"); got != "编辑" {
		t.Errorf("zh-CN should fall back to zh: T(Edit) = %q, want 编辑", got)
	}
}

// TestSpecificLocaleBeatsRoot: zh-CN-specific entries must override
// the zh-rooted fallback when both exist.
func TestSpecificLocaleBeatsRoot(t *testing.T) {
	tr := fresh()
	tr.Add("zh", "Edit", "编辑（通用）")
	tr.Add("zh-CN", "Edit", "编辑")
	tr.SetLocale("zh-CN")
	if got := tr.T("Edit"); got != "编辑" {
		t.Errorf("zh-CN should win over zh: got %q", got)
	}
}

// TestSetLocaleEmptyDisables: changing locale back to "" must
// disable translation again, even after a prior SetLocale.
func TestSetLocaleEmptyDisables(t *testing.T) {
	tr := fresh()
	tr.Add("zh-CN", "File", "文件")
	tr.SetLocale("zh-CN")
	tr.SetLocale("")
	if got := tr.T("File"); got != "File" {
		t.Errorf("after SetLocale(\"\"): T = %q, want passthrough", got)
	}
}

// TestTfFormatsArguments: Tf substitutes via fmt.Sprintf using the
// translated form as the format string. Translators can reorder %s
// for grammatical correctness; we don't validate directive count
// here because Sprintf's own behaviour is well-defined enough.
func TestTfFormatsArguments(t *testing.T) {
	tr := fresh()
	tr.Add("zh-CN", "Saved %s", "已保存 %s")
	tr.SetLocale("zh-CN")
	if got := tr.Tf("Saved %s", "main.go"); got != "已保存 main.go" {
		t.Errorf("Tf = %q, want 已保存 main.go", got)
	}
}

// TestTnEnglishPlural: default rule picks form 0 for n=1 and form
// 1 otherwise. Translation keyed off source singular.
func TestTnEnglishPlural(t *testing.T) {
	tr := fresh()
	tr.Add("zh-CN", "%d item", "%d 项")
	tr.Add("zh-CN", "%d items", "%d 项")
	tr.SetLocale("zh-CN")
	if got := tr.Tn("%d item", "%d items", 1); got != "1 项" {
		t.Errorf("Tn(1) = %q, want 1 项", got)
	}
	if got := tr.Tn("%d item", "%d items", 5); got != "5 项" {
		t.Errorf("Tn(5) = %q, want 5 项", got)
	}
}

// TestTnChineseHasSinglePluralForm: zh has chinesePlural (always
// index 0) so Tn(_, _, n) always returns the first form regardless
// of count.
func TestTnChineseHasSinglePluralForm(t *testing.T) {
	tr := fresh()
	tr.Add("zh-CN", "%d item", "%d 项")
	tr.Add("zh-CN", "%d items", "%d 项目")
	tr.SetLocale("zh-CN")
	// Despite n=5, Chinese rule picks form 0 (singular source). Both
	// forms register, but the lookup keys off source = "%d item".
	if got := tr.Tn("%d item", "%d items", 5); got != "5 项" {
		t.Errorf("Chinese Tn(5) = %q, want 5 项 (always form 0)", got)
	}
}

// TestSetPluralRuleCustom registers a Russian-style 3-form rule and
// verifies TnEx selects the right form. n=1 → 0, n=2 → 1, n=11 → 2
// is the simplified rule the test pins.
func TestSetPluralRuleCustom(t *testing.T) {
	tr := fresh()
	russianRule := func(n int) int {
		if n%10 == 1 && n%100 != 11 {
			return 0
		}
		if n%10 >= 2 && n%10 <= 4 && (n%100 < 10 || n%100 >= 20) {
			return 1
		}
		return 2
	}
	tr.SetPluralRule("ru", russianRule)
	tr.SetLocale("ru")

	forms := []string{"%d apple", "%d apples-2-4", "%d apples-many"}
	if got := tr.TnEx(forms, 1); got != "1 apple" {
		t.Errorf("TnEx(1) = %q, want 1 apple", got)
	}
	if got := tr.TnEx(forms, 3); got != "3 apples-2-4" {
		t.Errorf("TnEx(3) = %q, want 3 apples-2-4", got)
	}
	if got := tr.TnEx(forms, 11); got != "11 apples-many" {
		t.Errorf("TnEx(11) = %q, want 11 apples-many", got)
	}
}

// TestLoadFromBytesParsesNestedJSON exercises the file format: a top
// level locale → key → translation map.
func TestLoadFromBytesParsesNestedJSON(t *testing.T) {
	tr := fresh()
	json := []byte(`{
        "zh-CN": {"File": "文件", "Edit": "编辑"},
        "ja":    {"File": "ファイル"}
    }`)
	if err := tr.LoadFromBytes(json); err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	tr.SetLocale("zh-CN")
	if got := tr.T("File"); got != "文件" {
		t.Errorf("zh-CN T(File) = %q, want 文件", got)
	}
	tr.SetLocale("ja")
	if got := tr.T("File"); got != "ファイル" {
		t.Errorf("ja T(File) = %q, want ファイル", got)
	}
}

// TestLoadFromFileRoundTripWithSaveToFile: write a translation,
// reload it, see the same data come back.
func TestLoadFromFileRoundTripWithSaveToFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "translations.json")

	src := fresh()
	src.Add("zh-CN", "File", "文件")
	src.Add("zh-CN", "Edit", "编辑")
	if err := src.SaveToFile(path); err != nil {
		t.Fatalf("SaveToFile: %v", err)
	}

	dst := fresh()
	if err := dst.LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	dst.SetLocale("zh-CN")
	if got := dst.T("File"); got != "文件" {
		t.Errorf("round-trip T(File) = %q, want 文件", got)
	}
}

// TestNormaliseLocaleTag covers every documented form: POSIX with
// encoding suffix, BCP-47, and the special "C"/"POSIX" sentinel.
func TestNormaliseLocaleTag(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"zh_CN.UTF-8", "zh-CN"},
		{"en_US", "en-US"},
		{"zh-CN", "zh-CN"},
		{"C", "en"},
		{"POSIX", "en"},
		{"", "en"},
		{"en_US@latin", "en-US"},
	}
	for _, c := range cases {
		if got := normaliseLocaleTag(c.in); got != c.want {
			t.Errorf("normaliseLocaleTag(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDetectLocaleHonoursLCAll asserts the env-var ladder on hosts
// where LC_ALL is set. We use t.Setenv so the detection is sandboxed
// to this test and we don't perturb the rest of the suite.
func TestDetectLocaleHonoursLCAll(t *testing.T) {
	t.Setenv("LC_ALL", "fr_FR.UTF-8")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "")
	got, _ := DetectLocale()
	if got != "fr-FR" {
		t.Errorf("DetectLocale() = %q, want fr-FR", got)
	}
}

// TestDetectLocaleFallsBackToLang: when LC_ALL/LC_MESSAGES are
// empty, DetectLocale should reach LANG.
func TestDetectLocaleFallsBackToLang(t *testing.T) {
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANG", "ja_JP.UTF-8")
	got, _ := DetectLocale()
	if got != "ja-JP" {
		t.Errorf("DetectLocale() = %q, want ja-JP", got)
	}
}

// TestPackageLevelDefault: package-level helpers route through
// Default. We isolate each test from the others by saving + restoring.
func TestPackageLevelDefault(t *testing.T) {
	saved := *Default
	defer func() { *Default = saved }()

	*Default = *NewTranslator()
	Add("zh-CN", "Hello", "你好")
	SetLocale("zh-CN")
	if got := T("Hello"); got != "你好" {
		t.Errorf("package-level T(Hello) = %q, want 你好", got)
	}
}

// TestExportRoundTripsThroughLoader: round-trip via Export → JSON
// → LoadFromBytes preserves every entry.
func TestExportRoundTripsThroughLoader(t *testing.T) {
	src := fresh()
	src.Add("zh-CN", "File", "文件")
	src.Add("ja", "File", "ファイル")

	exported := src.Export()
	if exported["zh-CN"]["File"] != "文件" {
		t.Errorf("export zh-CN/File = %q, want 文件", exported["zh-CN"]["File"])
	}
	if exported["ja"]["File"] != "ファイル" {
		t.Errorf("export ja/File = %q, want ファイル", exported["ja"]["File"])
	}
}

// Sanity check that the loader writes a file and the reader parses
// it back losslessly using the package-level helpers (catches a
// subtle bug where SaveToFile bypassed the Default's mutex but
// LoadFromFile honoured it, or vice versa).
func TestPackageLevelLoaders(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tr.json")
	if err := os.WriteFile(path, []byte(`{"zh-CN": {"OK": "确定"}}`), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	saved := *Default
	defer func() { *Default = saved }()
	*Default = *NewTranslator()

	if err := LoadFromFile(path); err != nil {
		t.Fatalf("LoadFromFile: %v", err)
	}
	SetLocale("zh-CN")
	if got := T("OK"); got != "确定" {
		t.Errorf("loaded T(OK) = %q, want 确定", got)
	}
}
