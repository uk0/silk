package gui

import "github.com/uk0/silk/i18n"

// Register SDK defaults once; hosts may override them after importing gui.
// Menus translate on each opening, never on each paint or idle tick.
func init() {
	i18n.AddMany("zh", map[string]string{
		"Undo": "撤销", "Redo": "重做", "Cut": "剪切", "Copy": "复制", "Paste": "粘贴", "Delete": "删除", "Select All": "全选",
		"Add Cursor Above": "向上添加光标", "Add Cursor Below": "向下添加光标", "Keep Primary Cursor": "仅保留主光标",
		"Rename Symbol": "重命名符号", "Go to Definition": "跳转到定义", "Find References": "查找引用", "Go to Type Definition": "跳转到类型定义", "Go to Implementation": "跳转到实现", "New name:": "新名称：",
	})
}
