package settings

import (
	"strconv"
	"strings"
)

// Typed convenience accessors. Each pair (T(key, def), SetT(key, v))
// matches the QSettings idiom — read with a typed default, write with
// the natural Go type. Callers can mix Value/SetValue (untyped) with
// these when they want the runtime to handle the parsing.
//
// Defaults: missing key OR parse failure returns the supplied default.
// Parse failures are silent because settings files are user-editable
// and a corrupt entry shouldn't abort the app — fall back to the
// default and let Sync() clean up next save.

// Bool reads key as a boolean. "true"/"True"/"1" → true; "false"/
// "False"/"0" → false. Anything else returns the default.
func (s *Settings) Bool(key string, def bool) bool {
	v := s.Value(key)
	if v == nil {
		return def
	}
	str, ok := v.(string)
	if !ok {
		return def
	}
	switch strings.ToLower(str) {
	case "true", "1", "yes", "y":
		return true
	case "false", "0", "no", "n":
		return false
	}
	return def
}

// SetBool writes a boolean as "true" / "false". Mirrors Qt's
// QSettings::setValue with a Bool — the canonical lowercase form so
// the persisted file is human-readable.
func (s *Settings) SetBool(key string, v bool) error {
	return s.SetValue(key, v)
}

// Int reads key as a 64-bit integer.
func (s *Settings) Int(key string, def int64) int64 {
	v := s.Value(key)
	if v == nil {
		return def
	}
	str, ok := v.(string)
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(strings.TrimSpace(str), 10, 64)
	if err != nil {
		return def
	}
	return n
}

// SetInt writes a 64-bit integer.
func (s *Settings) SetInt(key string, v int64) error {
	return s.SetValue(key, v)
}

// Float64 reads key as a float64.
func (s *Settings) Float64(key string, def float64) float64 {
	v := s.Value(key)
	if v == nil {
		return def
	}
	str, ok := v.(string)
	if !ok {
		return def
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(str), 64)
	if err != nil {
		return def
	}
	return f
}

// SetFloat64 writes a float64.
func (s *Settings) SetFloat64(key string, v float64) error {
	return s.SetValue(key, v)
}

// String reads key as a string. The default form (one of two) returns
// def when the key is missing.
func (s *Settings) String(key string, def string) string {
	v := s.Value(key)
	if v == nil {
		return def
	}
	if str, ok := v.(string); ok {
		return str
	}
	return def
}

// SetString writes a string verbatim.
func (s *Settings) SetString(key string, v string) error {
	return s.SetValue(key, v)
}

// StringList reads key as a comma-separated list of values, mirroring
// the Qt convention of storing QStringList as a comma-joined string.
// Empty value yields an empty (non-nil) slice; missing key yields the
// supplied default.
//
// Escape handling: a literal comma inside an item is double-comma
// encoded ("a,b,c" → ["a", "b", "c"]; "a,,b" → ["a,b"]). This matches
// Qt's QSettings format on the INI backend.
func (s *Settings) StringList(key string, def []string) []string {
	v := s.Value(key)
	if v == nil {
		return def
	}
	str, ok := v.(string)
	if !ok {
		return def
	}
	return decodeStringList(str)
}

// SetStringList writes a string slice using the comma encoding above.
func (s *Settings) SetStringList(key string, v []string) error {
	return s.SetValue(key, encodeStringList(v))
}

func encodeStringList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	for i, it := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		// Double any literal commas in the item.
		b.WriteString(strings.ReplaceAll(it, ",", ",,"))
	}
	return b.String()
}

func decodeStringList(s string) []string {
	if s == "" {
		return []string{}
	}
	// Split on single commas, where double-comma is the literal-comma
	// escape. Walk the string character by character to handle the
	// escape correctly.
	var (
		out   []string
		curr  strings.Builder
		runes = []rune(s)
	)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c == ',' {
			if i+1 < len(runes) && runes[i+1] == ',' {
				curr.WriteRune(',')
				i++
				continue
			}
			out = append(out, curr.String())
			curr.Reset()
			continue
		}
		curr.WriteRune(c)
	}
	out = append(out, curr.String())
	return out
}
