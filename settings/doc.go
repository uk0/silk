// Package settings is Silk's preferences-storage runtime — the
// equivalent of Qt's QSettings. It wraps a TDoc-backed file with a
// Qt-style key/value API: hierarchical groups, typed getters with
// default values, optional ordered arrays, and an explicit Sync()
// flush.
//
// Why a TDoc backend instead of native registry / plist? Three reasons:
//
//  1. Cross-platform parity. The same .silkui-style file format works
//     on macOS, Linux, Windows; users who copy ~/.config/<app> between
//     machines see settings carry over.
//  2. Re-uses Silk's existing persistence layer. TDoc already handles
//     numeric types, strings, lists, nested structures — no JSON or
//     INI library to add.
//  3. Editable by hand. Plist is binary on modern macOS; the registry
//     is opaque. A TDoc text file is grep-friendly, diffable, and
//     can be hand-edited when the app is misconfigured.
//
// Typical usage:
//
//	import "silk/settings"
//
//	s := settings.Default("MyOrg", "MyApp")
//	s.SetValue("editor/fontSize", 14)
//	size := s.Int("editor/fontSize", 12) // 12 if not set
//	s.Sync()
//
// Group syntax: SetValue("group/sub/key", v) writes to the nested
// path. BeginGroup("group") makes subsequent calls relative to that
// prefix:
//
//	s.BeginGroup("editor")
//	s.SetValue("fontSize", 14)        // → editor/fontSize
//	s.SetValue("theme", "dark")       // → editor/theme
//	s.EndGroup()
//
// Goal lined up against QSettings: feature-parity for the dominant 90%
// of API calls (Value, SetValue, Contains, Remove, AllKeys, BeginGroup,
// EndGroup, beginReadArray, beginWriteArray, Sync, Status). The Qt
// Format / Scope / Organization / Application metadata is NOT modelled
// — Silk's TDoc-based path resolution covers the same ground in one
// pass.
package settings
