package ged

import "github.com/uk0/silk/gui"

// ShortcutDef is one documented key binding: the chord that runs it, what it
// does, and where it applies. The 快捷键 panel is drawn from this table and
// from nothing else — a binding absent here is one no user can discover.
//
// Command is the identifier ged.KeyMap rebinds, so the two tables are checked
// against each other by TestDesignerShortcutsAgreeWithKeymap instead of being
// kept in step by hand. Context matches KeyBinding.Context: "global" bindings
// are the ones GedView.OnKeyDown and the designer frame implement, "editor"
// ones belong to the focused code editor.
type ShortcutDef struct {
	Command  string
	Context  string // "global" or "editor", as in KeyBinding.Context
	Category string // help-panel group heading
	Label    string // what the binding does
	Mods     uint8  // gui.ModAction / gui.ModShift / gui.ModAlt bitmap
	Key      int    // gui.Key* constant, or an upper-case ASCII rune
	// Chord names a family of keys that Key cannot address on its own (the four
	// arrows). Set it only in that case; everywhere else the chord is rendered
	// from Mods and Key so the panel cannot disagree with the binding.
	Chord string
}

// chord returns the printable chord for the binding.
func (s ShortcutDef) chord() string {
	if s.Chord != "" {
		return s.Chord
	}
	return chordString(s.Mods, s.Key)
}

// designerShortcuts lists every binding the designer declares, in the order the
// 快捷键 panel shows them.
//
// ModAction renders as "Ctrl" on every platform, macOS included, because
// GedView.OnKeyDown probes the physical Ctrl key (gui.IsKeyDown(gui.KeyCtrl))
// rather than the platform action modifier. Printing "Cmd" on macOS would
// advertise a chord the canvas does not answer.
var designerShortcuts = []ShortcutDef{
	// 文件
	{Command: "file.new", Context: "global", Category: "文件", Label: "新建", Mods: gui.ModAction, Key: 'N'},
	{Command: "file.open", Context: "global", Category: "文件", Label: "打开", Mods: gui.ModAction, Key: 'O'},
	{Command: "file.save", Context: "global", Category: "文件", Label: "保存设计", Mods: gui.ModAction, Key: 'S'},
	{Command: "file.saveAs", Context: "global", Category: "文件", Label: "另存为", Mods: gui.ModAction | gui.ModShift, Key: 'S'},
	{Command: "file.quickOpen", Context: "global", Category: "文件", Label: "快速打开文件", Mods: gui.ModAction, Key: 'P'},

	// 选择 — reaching a widget at all without touching the mouse.
	{Command: "design.nextWidget", Context: "global", Category: "选择", Label: "选中下一个控件", Key: gui.KeyTab},
	{Command: "design.prevWidget", Context: "global", Category: "选择", Label: "选中上一个控件", Mods: gui.ModShift, Key: gui.KeyTab},
	{Command: "edit.selectAll", Context: "global", Category: "选择", Label: "全选窗体上的控件", Mods: gui.ModAction, Key: 'A'},
	{Command: "edit.deselect", Context: "global", Category: "选择", Label: "取消选中", Key: gui.KeyEsc},

	// 编辑
	{Command: "edit.copy", Context: "global", Category: "编辑", Label: "复制", Mods: gui.ModAction, Key: 'C'},
	{Command: "edit.paste", Context: "global", Category: "编辑", Label: "粘贴", Mods: gui.ModAction, Key: 'V'},
	{Command: "edit.cut", Context: "global", Category: "编辑", Label: "剪切", Mods: gui.ModAction, Key: 'X'},
	{Command: "edit.duplicate", Context: "global", Category: "编辑", Label: "创建副本", Mods: gui.ModAction, Key: 'D'},
	{Command: "edit.delete", Context: "global", Category: "编辑", Label: "删除选中控件", Key: gui.KeyDelete},
	{Command: "design.rename", Context: "global", Category: "编辑", Label: "重命名选中控件", Key: gui.KeyF2},
	{Command: "edit.undo", Context: "global", Category: "编辑", Label: "撤销", Mods: gui.ModAction, Key: 'Z'},
	{Command: "edit.redo", Context: "global", Category: "编辑", Label: "重做", Mods: gui.ModAction, Key: 'Y'},

	// 布局
	{Command: "design.nudge", Context: "global", Category: "布局", Label: "移动选中控件(Shift 按网格步长)", Chord: "方向键"},
	{Command: "design.resize", Context: "global", Category: "布局", Label: "调整选中控件大小", Chord: "Ctrl+方向键"},
	{Command: "design.alignLeft", Context: "global", Category: "布局", Label: "左对齐", Mods: gui.ModAlt, Key: 'L'},
	{Command: "design.alignRight", Context: "global", Category: "布局", Label: "右对齐", Mods: gui.ModAlt, Key: 'R'},
	{Command: "design.alignTop", Context: "global", Category: "布局", Label: "顶对齐", Mods: gui.ModAlt, Key: 'T'},
	{Command: "design.alignBottom", Context: "global", Category: "布局", Label: "底对齐", Mods: gui.ModAlt, Key: 'B'},
	{Command: "design.alignCenterH", Context: "global", Category: "布局", Label: "水平居中对齐", Mods: gui.ModAlt, Key: 'C'},
	{Command: "design.alignCenterV", Context: "global", Category: "布局", Label: "垂直居中对齐", Mods: gui.ModAlt, Key: 'M'},
	{Command: "design.distributeH", Context: "global", Category: "布局", Label: "水平分布", Mods: gui.ModAlt, Key: 'H'},
	{Command: "design.distributeV", Context: "global", Category: "布局", Label: "垂直分布", Mods: gui.ModAlt, Key: 'V'},

	// 视图
	{Command: "view.design", Context: "global", Category: "视图", Label: "设计模式", Mods: gui.ModAction, Key: '1'},
	{Command: "view.code", Context: "global", Category: "视图", Label: "代码模式", Mods: gui.ModAction, Key: '2'},
	{Command: "view.zoomIn", Context: "global", Category: "视图", Label: "放大", Mods: gui.ModAction, Key: '='},
	{Command: "view.zoomOut", Context: "global", Category: "视图", Label: "缩小", Mods: gui.ModAction, Key: '-'},

	// 运行
	{Command: "run.compile", Context: "global", Category: "运行", Label: "编译运行", Key: gui.KeyF5},
	{Command: "run.preview", Context: "global", Category: "运行", Label: "预览窗体", Mods: gui.ModAction, Key: 'R'},
	{Command: "debug.perfOverlay", Context: "global", Category: "运行", Label: "性能浮层", Key: gui.KeyF12},

	// 帮助 — registered on the frame (gui.RegisterShortcut in design.go), so it
	// works wherever focus happens to be.
	{Command: "help.shortcuts", Context: "global", Category: "帮助", Label: "快捷键参考", Key: gui.KeyF1},

	// 代码 — the focused code editor, not the canvas.
	{Command: "editor.find", Context: "editor", Category: "代码", Label: "查找", Mods: gui.ModAction, Key: 'F'},
	{Command: "editor.gotoLine", Context: "editor", Category: "代码", Label: "跳转到行", Mods: gui.ModAction, Key: 'G'},
	{Command: "editor.gotoSymbol", Context: "editor", Category: "代码", Label: "跳转到符号", Mods: gui.ModAction | gui.ModShift, Key: 'O'},
	{Command: "editor.comment", Context: "editor", Category: "代码", Label: "注释/取消注释", Mods: gui.ModAction, Key: '/'},
	{Command: "editor.duplicateLine", Context: "editor", Category: "代码", Label: "复制当前行", Mods: gui.ModAction, Key: 'D'},
	{Command: "editor.format", Context: "editor", Category: "代码", Label: "格式化代码", Mods: gui.ModAction | gui.ModShift, Key: 'F'},
	{Command: "editor.rename", Context: "editor", Category: "代码", Label: "重命名符号", Mods: gui.ModAction | gui.ModShift, Key: 'R'},
	{Command: "editor.completion", Context: "editor", Category: "代码", Label: "代码补全", Mods: gui.ModAction, Key: gui.KeySpace},
	{Command: "editor.bookmark", Context: "editor", Category: "代码", Label: "添加/移除书签", Mods: gui.ModAction, Key: 'B'},
	{Command: "editor.nextBookmark", Context: "editor", Category: "代码", Label: "下一个书签", Key: gui.KeyF2},
	{Command: "editor.prevBookmark", Context: "editor", Category: "代码", Label: "上一个书签", Mods: gui.ModShift, Key: gui.KeyF2},
	{Command: "editor.addCursorAbove", Context: "editor", Category: "代码", Label: "向上添加光标", Mods: gui.ModAction | gui.ModAlt, Key: gui.KeyUp},
	{Command: "editor.addCursorBelow", Context: "editor", Category: "代码", Label: "向下添加光标", Mods: gui.ModAction | gui.ModAlt, Key: gui.KeyDown},
	{Command: "editor.navBack", Context: "editor", Category: "代码", Label: "后退", Mods: gui.ModAlt, Key: gui.KeyLeft},
	{Command: "editor.navForward", Context: "editor", Category: "代码", Label: "前进", Mods: gui.ModAlt, Key: gui.KeyRight},
}

// chordString renders a modifier bitmap and key as the chord users read, in the
// order ged.KeyMap already writes them ("Ctrl+Shift+S", "Ctrl+Alt+Up"). Returns
// "" for a zero key, which only a Chord-override entry has.
func chordString(mods uint8, key int) string {
	if key == 0 {
		return ""
	}
	out := ""
	if mods&gui.ModAction != 0 {
		out += "Ctrl+"
	}
	if mods&gui.ModShift != 0 {
		out += "Shift+"
	}
	if mods&gui.ModAlt != 0 {
		out += "Alt+"
	}
	return out + keyLabel(key)
}

// keyLabel names a single key. Only the keys designerShortcuts uses are spelled
// out; anything else is the ASCII character the binding matches literally.
func keyLabel(key int) string {
	switch key {
	case gui.KeyTab:
		return "Tab"
	case gui.KeyEsc:
		return "Esc"
	case gui.KeyDelete:
		return "Delete"
	case gui.KeyBackSpace:
		return "Backspace"
	case gui.KeySpace:
		return "Space"
	case gui.KeyLeft:
		return "Left"
	case gui.KeyRight:
		return "Right"
	case gui.KeyUp:
		return "Up"
	case gui.KeyDown:
		return "Down"
	case gui.KeyF1:
		return "F1"
	case gui.KeyF2:
		return "F2"
	case gui.KeyF5:
		return "F5"
	case gui.KeyF12:
		return "F12"
	}
	return string(rune(key))
}

// shortcutCategories groups designerShortcuts for the help panel, preserving
// declaration order both between and inside groups.
func shortcutCategories() []shortcutCategory {
	var out []shortcutCategory
	at := make(map[string]int, len(designerShortcuts))
	for _, sc := range designerShortcuts {
		i, ok := at[sc.Category]
		if !ok {
			out = append(out, shortcutCategory{name: sc.Category})
			i = len(out) - 1
			at[sc.Category] = i
		}
		out[i].shortcuts = append(out[i].shortcuts, shortcutEntry{sc.chord(), sc.Label})
	}
	return out
}
