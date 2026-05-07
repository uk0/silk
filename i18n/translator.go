package i18n

import (
	"fmt"
	"strings"
	"sync"
)

// Translator holds a per-locale translation table and is the unit of
// translation in this package. The package-level helpers (T, Tn, Tf,
// SetLocale, LoadFromFile, ...) all delegate to a global Default
// translator; tests and multi-tenant runtimes can construct their own
// via NewTranslator.
//
// Synchronisation: every Translator is internally locked with a RWMutex
// so callers from goroutines other than the UI thread see a consistent
// view. Reads (T/Tf/Tn) are concurrent; writes (SetLocale, Load*) are
// serialised. The lock cost is negligible compared to the map lookup.
type Translator struct {
	mu sync.RWMutex

	// locale is the active translation target. An empty string means
	// "no translation" — every lookup returns the source string. We
	// don't pre-pick a default at construction so applications can
	// detect the OS locale themselves before activating it.
	locale string

	// table is locale → source-string → translated-string. Lookup
	// walks: table[locale] then table[language(locale)] then source.
	// language() strips a "-XX" region suffix; "zh-CN" falls back to
	// "zh", which falls back to the source string.
	table map[string]map[string]string

	// pluralRules maps locale (or language root) to a pluralization
	// rule. Lookup picks the most specific rule that matches; falls
	// back to defaultPlural (English-like) when nothing is registered.
	pluralRules map[string]PluralRule
}

// PluralRule maps an integer count to a plural-form index. Index 0 is
// always the first form supplied to Tn; subsequent forms (index 1..)
// are passed via the variadic plurals slice on extended APIs (TnEx).
//
// The rule is a function so locales with non-trivial rules — Russian
// has 3 forms, Arabic has 6 — can be expressed without committing
// the package to a specific data format.
type PluralRule func(n int) int

// englishPlural is the dominant rule across European languages: 1
// uses singular, all other counts use plural. Used as the fallback
// when no rule is registered for the active locale.
func englishPlural(n int) int {
	if n == 1 {
		return 0
	}
	return 1
}

// chinesePlural collapses every count to a single form. CJK languages
// don't grammaticalise plurality; the same noun phrase covers any
// count. Translations supplied to Tn are re-used for every value.
func chinesePlural(n int) int { return 0 }

// NewTranslator creates an empty translator with no locale and no
// loaded translations. Callers populate via Load*, then activate a
// locale with SetLocale.
func NewTranslator() *Translator {
	t := &Translator{
		table:       make(map[string]map[string]string),
		pluralRules: make(map[string]PluralRule),
	}
	// Pre-register the two builtin rules. Custom rules can replace
	// them via SetPluralRule.
	t.pluralRules["zh"] = chinesePlural
	t.pluralRules["ja"] = chinesePlural
	t.pluralRules["ko"] = chinesePlural
	t.pluralRules["th"] = chinesePlural
	t.pluralRules["vi"] = chinesePlural
	return t
}

// SetLocale activates a locale by tag (e.g. "zh-CN", "en-US", "ja").
// Lookup misses fall back to the language root ("zh-CN" → "zh") and
// finally to the source string. An empty string disables translation.
func (t *Translator) SetLocale(locale string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.locale = locale
}

// Locale returns the active locale tag, or "" when translation is off.
func (t *Translator) Locale() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.locale
}

// SetPluralRule registers a custom pluralization rule for a locale or
// language root. "ru" applies to all Russian variants ("ru", "ru-RU"),
// "ru-RU" is more specific. Lookup picks the most specific match.
func (t *Translator) SetPluralRule(locale string, rule PluralRule) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.pluralRules[locale] = rule
}

// Add inserts a single translation entry. Useful for tests or for
// programmatically-built translation tables — production code usually
// loads a whole locale via Load*. Overwrites any existing entry for
// the same (locale, source) pair.
func (t *Translator) Add(locale, source, translated string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.table[locale] == nil {
		t.table[locale] = make(map[string]string)
	}
	t.table[locale][source] = translated
}

// AddMany merges every entry from translations into the table for
// locale. Existing entries with the same key are overwritten. Used
// by the JSON loader to install a whole locale at once.
func (t *Translator) AddMany(locale string, translations map[string]string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.table[locale] == nil {
		t.table[locale] = make(map[string]string, len(translations))
	}
	for k, v := range translations {
		t.table[locale][k] = v
	}
}

// T returns the translation of source for the active locale, or
// source itself when no translation is registered. Argumentless —
// callers needing parameter substitution use Tf.
func (t *Translator) T(source string) string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lookup(source)
}

// Tf is T with format substitution. The translated form is the
// fmt.Sprintf format string, so translators can reorder %s arguments
// for syntactic correctness in their language. Format directives must
// match in count and type across locales — verifying that is the
// translator's job, not the runtime's.
func (t *Translator) Tf(source string, args ...interface{}) string {
	t.mu.RLock()
	format := t.lookup(source)
	t.mu.RUnlock()
	return fmt.Sprintf(format, args...)
}

// Tn picks between singular and plural forms based on n. Pass the
// English-style singular as source and plural as plural; both are
// looked up independently in the translation table. Languages with
// more than two forms call TnEx with the full list.
//
// The returned string has %d substituted by n. To embed n with a
// specific format directive, write that directive in source/plural
// (e.g. "%d items", "%5d items"); we just feed n to fmt.Sprintf.
func (t *Translator) Tn(source, plural string, n int) string {
	return t.TnEx([]string{source, plural}, n)
}

// TnEx is the multi-form Tn for languages like Russian or Arabic
// that have 3+ plural forms. forms is indexed by the locale's
// PluralRule output; the source-language singular at index 0 is
// also the lookup key into the translation table for every form.
func (t *Translator) TnEx(forms []string, n int) string {
	if len(forms) == 0 {
		return ""
	}
	t.mu.RLock()
	rule := t.pluralRule()
	idx := rule(n)
	if idx < 0 || idx >= len(forms) {
		idx = 0
	}
	source := forms[idx]
	translated := t.lookup(source)
	t.mu.RUnlock()
	return fmt.Sprintf(translated, n)
}

// lookup is the inner translation table walk. Caller must hold mu.RLock.
func (t *Translator) lookup(source string) string {
	if t.locale == "" {
		return source
	}
	if m := t.table[t.locale]; m != nil {
		if v, ok := m[source]; ok {
			return v
		}
	}
	if root := languageRoot(t.locale); root != t.locale {
		if m := t.table[root]; m != nil {
			if v, ok := m[source]; ok {
				return v
			}
		}
	}
	return source
}

// pluralRule returns the most specific PluralRule for the active
// locale. Caller must hold the lock.
func (t *Translator) pluralRule() PluralRule {
	if r := t.pluralRules[t.locale]; r != nil {
		return r
	}
	if root := languageRoot(t.locale); root != t.locale {
		if r := t.pluralRules[root]; r != nil {
			return r
		}
	}
	return englishPlural
}

// languageRoot strips a region suffix from a BCP-47 tag.
//
//	zh-CN → zh
//	en-US → en
//	en    → en
//	      →  (empty input falls through)
func languageRoot(locale string) string {
	if i := strings.IndexByte(locale, '-'); i >= 0 {
		return locale[:i]
	}
	if i := strings.IndexByte(locale, '_'); i >= 0 {
		// Tolerate POSIX-style "en_US" alongside BCP-47.
		return locale[:i]
	}
	return locale
}

// --- Package-level convenience over a Default translator. -----------
//
// Rationale for keeping a default global: every UI app has a single
// translator instance for its lifetime, and threading one through
// every widget callsite would bloat the API surface. Tests that need
// isolation use NewTranslator directly.

// Default is the singleton used by package-level helpers. Replace at
// startup if you need a custom configuration shared by all callers.
var Default = NewTranslator()

// SetLocale calls Default.SetLocale.
func SetLocale(locale string) { Default.SetLocale(locale) }

// Locale returns Default.Locale.
func Locale() string { return Default.Locale() }

// T calls Default.T.
func T(source string) string { return Default.T(source) }

// Tf calls Default.Tf.
func Tf(source string, args ...interface{}) string { return Default.Tf(source, args...) }

// Tn calls Default.Tn.
func Tn(source, plural string, n int) string { return Default.Tn(source, plural, n) }

// TnEx calls Default.TnEx.
func TnEx(forms []string, n int) string { return Default.TnEx(forms, n) }

// Add registers a single (locale, source, translated) entry on Default.
func Add(locale, source, translated string) { Default.Add(locale, source, translated) }

// AddMany merges a whole locale into Default. Convenience for tests
// and small embedded translation tables.
func AddMany(locale string, translations map[string]string) {
	Default.AddMany(locale, translations)
}
