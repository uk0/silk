// Package i18n is Silk's translation runtime — the equivalent of Qt's
// QTranslator + tr() pair. Source strings stay in Go code (so grep
// finds them, IDE refactors them, and the binary runs without any
// translation file at all). At runtime the translator looks up the
// source string in a per-locale map and returns the translated form.
// On a lookup miss it returns the source string unchanged, so a
// missing translation never produces a blank UI.
//
// Typical usage:
//
//	import "github.com/uk0/silk/i18n"
//
//	func init() {
//	    i18n.LoadFromFile("translations/zh-CN.json")
//	    i18n.SetLocale("zh-CN")
//	}
//
//	label := gui.NewLabel(i18n.T("File"))           // "文件"
//	status := i18n.Tf("Saved %s", filename)         // "已保存 main.go"
//	hint := i18n.Tn("%d item", "%d items", count)   // pluralized
//
// Goal lined up against Qt: Qt's tr() depends on the MOC compiler
// to embed source strings into a .ts file. Silk has no MOC and we
// don't want one — instead the translator just keys off the source
// string at runtime. xgettext-style tooling can scan source for
// i18n.T(...) calls and emit a starter translation table; we leave
// that to a future tool.
//
// File format: JSON, mapping locale → key → translated string. A
// real-world translations.json:
//
//	{
//	  "zh-CN": { "File": "文件", "Edit": "编辑" },
//	  "ja":    { "File": "ファイル" }
//	}
//
// Plurals: per-locale plural function maps integer count to a
// plural-form index. Two simple builtin rules cover most needs:
// English-like (1 / other) and Chinese-like (no plurals). Custom
// languages register their own via Translator.SetPluralRule.
package i18n
