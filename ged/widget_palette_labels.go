package ged

import "github.com/uk0/silk/i18n"

// Preserve the registration/category identity used by existing plugins. Only
// the presentation is translated; factory names and drag payloads never change.
var paletteCategoryKeys = map[string]string{
	"输入控件 (Input)":    "Input",
	"显示控件 (Display)":  "Display",
	"容器/布局 (Layout)":  "Containers and Layouts",
	"数据视图 (Data)":     "Data Views",
	"图表 (Charts)":     "Charts",
	"组态 (SCADA)":      "SCADA",
	"对话框/窗口 (Window)": "Dialogs and Windows",
}

func init() {
	i18n.AddMany("zh-CN", map[string]string{
		"Input": "输入控件", "Display": "显示控件", "Containers and Layouts": "容器与布局",
		"Data Views": "数据视图", "Charts": "图表", "SCADA": "组态", "Dialogs and Windows": "对话框与窗口",
	})
}

func paletteCategoryLabel(identity string) string {
	if key, ok := paletteCategoryKeys[identity]; ok {
		return i18n.T(key)
	}
	return i18n.T(identity)
}
