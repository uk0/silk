package gui

import "github.com/uk0/silk/i18n"

// Standard command IDs stay untranslated. Captions are resolved when buttons
// are built; custom captions remain the host's responsibility. Hosts may
// override these SDK defaults after importing gui, without per-frame lookups.
func init() {
	i18n.AddMany("zh", map[string]string{
		"OK": "确定", "Cancel": "取消", "Save": "保存", "Discard": "不保存",
		"Yes": "是", "No": "否", "Apply": "应用", "Save All": "全部保存",
		"Discard All": "全部不保存", "Close All": "全部关闭",
		"Enter a new identifier name.":                            "请输入新的标识符名称。",
		"Use a Go identifier, not a keyword or blank identifier.": "请使用 Go 标识符，不能是关键字或 _。",
	})
}
