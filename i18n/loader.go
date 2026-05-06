package i18n

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

// LoadFromBytes parses raw JSON content as a translation table and
// installs every locale into t. The expected shape is a single
// JSON object whose top-level keys are locale tags ("zh-CN", "en")
// and whose values are objects mapping source strings to translations.
//
//	{
//	  "zh-CN": { "File": "文件", "Edit": "编辑" },
//	  "ja":    { "File": "ファイル" }
//	}
//
// Existing entries with the same (locale, key) pair are overwritten.
// Locales not in the file are unaffected.
func (t *Translator) LoadFromBytes(data []byte) error {
	var raw map[string]map[string]string
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("i18n: parse json: %w", err)
	}
	for locale, entries := range raw {
		t.AddMany(locale, entries)
	}
	return nil
}

// LoadFromFile is LoadFromBytes that reads a file from disk first.
// The file is expected to be JSON encoded in UTF-8 (no BOM); other
// encodings need to be normalised by the caller.
func (t *Translator) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("i18n: read %s: %w", path, err)
	}
	return t.LoadFromBytes(data)
}

// Export returns the full translation table as a JSON-marshalable map,
// useful for tooling that wants to round-trip translations through
// memory or merge programmatic edits with file contents.
func (t *Translator) Export() map[string]map[string]string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[string]map[string]string, len(t.table))
	for locale, entries := range t.table {
		copyM := make(map[string]string, len(entries))
		for k, v := range entries {
			copyM[k] = v
		}
		out[locale] = copyM
	}
	return out
}

// SaveToFile writes Export's result to path as pretty-printed JSON.
// Useful in tooling that programmatically edits translations and
// wants to persist them. Production translation files are usually
// edited by humans, so this exists for tests and helpers, not as the
// primary authoring path.
func (t *Translator) SaveToFile(path string) error {
	data, err := json.MarshalIndent(t.Export(), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadFromBytes / LoadFromFile / Export / SaveToFile mirrors on the
// package-level Default translator. They are convenience wrappers so
// startup code can write `i18n.LoadFromFile(...)` instead of routing
// through Default explicitly.

// LoadFromBytes calls Default.LoadFromBytes.
func LoadFromBytes(data []byte) error { return Default.LoadFromBytes(data) }

// LoadFromFile calls Default.LoadFromFile.
func LoadFromFile(path string) error { return Default.LoadFromFile(path) }

// --- Locale detection -------------------------------------------------

// DetectLocale returns the host's preferred locale, in priority order:
//
//  1. LC_ALL  — POSIX override, beats every other env var.
//  2. LC_MESSAGES — POSIX message-catalog locale (the closest analogue
//     of the i18n.T target).
//  3. LANG — POSIX user-default locale.
//
// On macOS, we additionally consult `defaults read -g AppleLocale`
// because the env vars there are typically empty unless the user is
// running from a properly-configured shell. The macOS check is best-
// effort; failure surfaces as the env-var fallback below.
//
// Returns ("en", nil) when nothing is set — a sensible neutral
// default rather than "" so callers can pass the result straight to
// SetLocale and start translating.
func DetectLocale() (string, error) {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(key); v != "" {
			return normaliseLocaleTag(v), nil
		}
	}
	// macOS-specific fallback: the global AppleLocale preference.
	if loc, ok := readMacAppleLocale(); ok {
		return normaliseLocaleTag(loc), nil
	}
	return "en", errors.New("i18n: no locale env var set; defaulting to en")
}

// normaliseLocaleTag strips POSIX encoding suffixes and converts the
// underscore separator to a hyphen so the result matches BCP-47 tags
// the rest of i18n uses internally.
//
//	zh_CN.UTF-8 → zh-CN
//	en_US       → en-US
//	C           → en
//	POSIX       → en
func normaliseLocaleTag(s string) string {
	if i := indexByte(s, '.'); i >= 0 {
		s = s[:i]
	}
	if i := indexByte(s, '@'); i >= 0 {
		// "zh_CN@latin" → drop the modifier; Silk has no modifier
		// concept and the matching translation key is the bare tag.
		s = s[:i]
	}
	for i, c := range s {
		if c == '_' {
			s = s[:i] + "-" + s[i+1:]
		}
	}
	switch s {
	case "C", "POSIX", "":
		return "en"
	}
	return s
}

func indexByte(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
