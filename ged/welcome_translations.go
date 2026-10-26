package ged

import "github.com/uk0/silk/i18n"

func registerWelcomeTranslations() {
	i18n.AddMany("zh-CN", map[string]string{
		"Welcome": "欢迎", "New Project": "新建项目", "New Form": "新建窗体", "Open File": "打开文件", "Save": "保存",
		"Keyboard shortcuts": "键盘快捷键", "Recent files": "最近打开", "No recent files yet": "暂无最近文件", "Open a file to get started.": "打开文件，开始创作。",
		"A visual designer for Go interfaces.": "面向 Go 界面的可视化设计器。", "Go editing and visual interface design.": "Go 代码编辑与可视化界面设计。", "Drag, arrange and generate Go.": "拖拽控件、调整布局并生成 Go 代码。",
		"Quick Open": "快速打开", "Command Palette": "命令面板", "Preview": "预览", "Run": "运行",
	})
}
