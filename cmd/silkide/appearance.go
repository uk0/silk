package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

type ideAppearance struct {
	InsertSpaces                                             bool
	Locale, Theme, UIFont, EditorFont                        string
	UISize, EditorSize, TabSize                              int
	Accent, Background, Foreground, Keyword, Comment, String string
}

func defaultIDEAppearance() ideAppearance {
	family := "monospace"
	if runtime.GOOS == "windows" {
		family = "Consolas"
	}
	if runtime.GOOS == "darwin" {
		family = "Menlo"
	}
	return ideAppearance{Locale: "en", Theme: "dark", UIFont: gui.Theme().Font.Family(), EditorFont: family, UISize: 14, EditorSize: 14, TabSize: 4}
}

func (p *preferences) Appearance() ideAppearance {
	a := defaultIDEAppearance()
	a.InsertSpaces = p.store.Bool("appearance/insertSpaces", false)
	for key, ptr := range map[string]*string{"locale": &a.Locale, "theme": &a.Theme, "uiFont": &a.UIFont, "editorFont": &a.EditorFont, "accent": &a.Accent, "background": &a.Background, "foreground": &a.Foreground, "keyword": &a.Keyword, "comment": &a.Comment, "string": &a.String} {
		*ptr = p.store.String("appearance/"+key, *ptr)
	}
	for key, ptr := range map[string]*int{"uiSize": &a.UISize, "editorSize": &a.EditorSize, "tabSize": &a.TabSize} {
		*ptr = int(p.store.Int("appearance/"+key, int64(*ptr)))
	}
	if a.validate() != nil {
		return defaultIDEAppearance()
	}
	return a
}

func parseAppearanceColor(s string) (*paint.Color, error) {
	if s == "" {
		return nil, nil
	}
	if len(s) != 7 || s[0] != '#' {
		return nil, fmt.Errorf("expected #RRGGBB: %q", s)
	}
	v, err := strconv.ParseUint(s[1:], 16, 24)
	if err != nil {
		return nil, err
	}
	return &paint.Color{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}, nil
}
func (a ideAppearance) validate() error {
	if a.Locale != "en" && a.Locale != "zh-CN" {
		return fmt.Errorf("unsupported language")
	}
	if a.Theme != "dark" && a.Theme != "light" {
		return fmt.Errorf("unsupported theme")
	}
	if a.UISize < 10 || a.UISize > 28 || a.EditorSize < 8 || a.EditorSize > 40 || a.TabSize < 1 || a.TabSize > 16 {
		return fmt.Errorf("UI size: 10-28, editor size: 8-40, tab width: 1-16")
	}
	for _, f := range []string{a.UIFont, a.EditorFont} {
		if strings.TrimSpace(f) == "" || len(f) > 128 || strings.ContainsAny(f, "\r\n\x00") {
			return fmt.Errorf("invalid font family")
		}
	}
	for _, s := range []string{a.Accent, a.Background, a.Foreground, a.Keyword, a.Comment, a.String} {
		if _, err := parseAppearanceColor(s); err != nil {
			return err
		}
	}
	return nil
}
func (p *preferences) SetAppearance(a ideAppearance) error {
	if err := a.validate(); err != nil {
		return err
	}
	for key, v := range map[string]string{"locale": a.Locale, "theme": a.Theme, "uiFont": a.UIFont, "editorFont": a.EditorFont, "accent": a.Accent, "background": a.Background, "foreground": a.Foreground, "keyword": a.Keyword, "comment": a.Comment, "string": a.String} {
		if err := p.store.SetString("appearance/"+key, v); err != nil {
			return err
		}
	}
	for key, v := range map[string]int{"uiSize": a.UISize, "editorSize": a.EditorSize, "tabSize": a.TabSize} {
		if err := p.store.SetInt("appearance/"+key, int64(v)); err != nil {
			return err
		}
	}
	if err := p.store.SetBool("appearance/insertSpaces", a.InsertSpaces); err != nil {
		return err
	}
	return p.requestSave()
}
func applyEditorAppearance(ed *gui.CodeEditor, a ideAppearance) {
	ed.SetFont(paint.NewFont(a.EditorFont, a.EditorSize, false, false))
	ed.SetTabSize(a.TabSize)
	ed.SetInsertSpaces(a.InsertSpaces)
	bg, _ := parseAppearanceColor(a.Background)
	fg, _ := parseAppearanceColor(a.Foreground)
	kw, _ := parseAppearanceColor(a.Keyword)
	co, _ := parseAppearanceColor(a.Comment)
	st, _ := parseAppearanceColor(a.String)
	ed.SetColors(gui.EditorColors{Background: bg, Foreground: fg, Keyword: kw, Comment: co, String: st})
}
func applyIDEAppearance(a ideAppearance) {
	mode := gui.ThemeDark
	if a.Theme == "light" {
		mode = gui.ThemeLight
	}
	gui.SetThemeMode(mode)
	gui.SetUIFont(paint.NewFont(a.UIFont, a.UISize, false, false))
	if a.Accent != "" {
		_ = gui.Theme().LoadStyleSheet("Frame { highlight: " + a.Accent + "; }")
	}
	for _, ed := range openEditors {
		if ed != nil {
			applyEditorAppearance(ed, a)
		}
	}
	if globalFrame != nil {
		globalFrame.Layout()
		globalFrame.Update()
	}
}

func appearanceLocale(p *preferences) string {
	if v := os.Getenv("SILKIDE_LOCALE"); v != "" {
		return v
	}
	if p != nil {
		return p.Appearance().Locale
	}
	return "en"
}

func buildAppearanceDialog(parent gui.IWidget, a ideAppearance) (*gui.Dialog, func() (ideAppearance, error)) {
	d := gui.NewDialog(i18n.T("Appearance & Editor"), parent)
	form := gui.NewFormLayout()
	labelWidth := math.Max(240, gui.Theme().Font.TextExtents(i18n.T("Language (restart required)")).XAdvance+16)
	form.SetLabelWidth(labelWidth)
	rowHeight := math.Max(32, gui.Theme().Font.FontExtents().Height+12)
	form.SetRowHeight(rowHeight)
	language := gui.NewComboBox()
	language.SetItems([]string{"English", "简体中文"})
	if a.Locale == "zh-CN" {
		language.SetActiveIndex(1)
	} else {
		language.SetActiveIndex(0)
	}
	form.AddRow(i18n.T("Language (restart required)"), language)
	theme := gui.NewComboBox()
	theme.SetItems([]string{i18n.T("Dark"), i18n.T("Light")})
	if a.Theme == "light" {
		theme.SetActiveIndex(1)
	} else {
		theme.SetActiveIndex(0)
	}
	form.AddRow(i18n.T("Theme"), theme)
	fields := map[string]*gui.Edit{}
	add := func(key, label, value string) {
		e := gui.NewEdit()
		e.SetText(value)

		fields[key] = e
		form.AddRow(i18n.T(label), e)
	}
	add("uiFont", "UI font", a.UIFont)
	add("uiSize", "UI font size", strconv.Itoa(a.UISize))
	add("editorFont", "Editor font", a.EditorFont)
	add("editorSize", "Editor font size", strconv.Itoa(a.EditorSize))
	add("tabSize", "Tab width", strconv.Itoa(a.TabSize))
	indentation := gui.NewComboBox()
	indentation.SetItems([]string{i18n.T("Tabs"), i18n.T("Spaces")})
	if a.InsertSpaces {
		indentation.SetActiveIndex(1)
	} else {
		indentation.SetActiveIndex(0)
	}
	form.AddRow(i18n.T("Indent using"), indentation)
	for _, v := range []struct{ k, l, v string }{{"accent", "Accent (#RRGGBB)", a.Accent}, {"background", "Editor background", a.Background}, {"foreground", "Editor text", a.Foreground}, {"keyword", "Keywords", a.Keyword}, {"comment", "Comments", a.Comment}, {"string", "Strings", a.String}} {
		add(v.k, v.l, v.v)
	}
	form.AddRow("", gui.NewLabel(i18n.T("Empty color = theme default; colors use #RRGGBB")))
	d.SetContent(form)
	d.AddButton(i18n.T("Apply"), gui.DialogOK)
	d.AddButton(i18n.T("Cancel"), gui.DialogCancel)
	d.SetSize(math.Max(740, labelWidth+480), math.Max(600, rowHeight*15+100))
	return d, func() (ideAppearance, error) {
		n := a
		n.InsertSpaces = indentation.ActiveIndex() == 1
		n.Locale = "en"
		if language.ActiveIndex() == 1 {
			n.Locale = "zh-CN"
		}
		n.Theme = "dark"
		if theme.ActiveIndex() == 1 {
			n.Theme = "light"
		}
		for key, ptr := range map[string]*string{"uiFont": &n.UIFont, "editorFont": &n.EditorFont, "accent": &n.Accent, "background": &n.Background, "foreground": &n.Foreground, "keyword": &n.Keyword, "comment": &n.Comment, "string": &n.String} {
			*ptr = strings.TrimSpace(fields[key].Text())
		}
		for key, ptr := range map[string]*int{"uiSize": &n.UISize, "editorSize": &n.EditorSize, "tabSize": &n.TabSize} {
			v, err := strconv.Atoi(fields[key].Text())
			if err != nil {
				return n, err
			}
			*ptr = v
		}
		return n, n.validate()
	}
}
func showAppearanceDialog(parent gui.IWidget) {
	if globalPrefs == nil {
		return
	}
	d, read := buildAppearanceDialog(parent, globalPrefs.Appearance())
	for d.ShowModal() == gui.DialogOK {
		a, err := read()
		if err == nil {
			err = globalPrefs.SetAppearance(a)
		}
		if err != nil {
			silkideToast(err.Error(), gui.ToastError)
			continue
		}
		applyIDEAppearance(a)
		silkideToast(i18n.T("Appearance applied; saving in background. Language changes apply after restart"), gui.ToastInfo)
		break
	}
}

// IDE-owned dock captions used to mix hardcoded Chinese SDK titles with an
// English menu. Normalize them here without changing reusable SDK defaults.
func localizeWorkbenchPanels() {
	if globalWorkbench != nil && globalWorkbench.welcome != nil {
		globalWorkbench.welcome.SetDockTitle(i18n.T("Welcome"))
	}
	i18n.AddMany("zh-CN", map[string]string{"Move Lines Up": "上移行", "Move Lines Down": "下移行", "Indent Lines": "增加行缩进", "Outdent Lines": "减少行缩进", "Indent using": "缩进方式", "Tabs": "制表符", "Spaces": "空格", "Files": "文件", "Search": "搜索", "Packages": "包列表", "Objects": "对象", "Outline": "大纲", "Bookmarks": "书签", "Terminal": "终端", "Output": "输出", "Problems": "问题", "Tests": "测试", "Log": "日志", "Debugger": "调试", "References": "引用", "Git Changes": "Git 更改", "TODO": "待办", "Git Log": "Git 历史"})
	for view, label := range map[gui.IWidget]string{globalFileExplorer: "Files", globalSearch: "Search", globalPackages: "Packages", globalInspector: "Objects", globalOutline: "Outline", globalBookmarks: "Bookmarks", globalTerminal: "Terminal", globalBuildOutput: "Output", globalProblems: "Problems", globalTestResults: "Tests", globalLog: "Log", globalDebugPanel: "Debugger", globalReferencesPanel: "References", globalGitChanges: "Git Changes", globalTodoPanel: "TODO", globalGitLog: "Git Log"} {
		if !nilWidget(view) {
			if titled, ok := view.(interface{ SetDockTitle(string) }); ok {
				titled.SetDockTitle(i18n.T(label))
			}
		}
	}
}
