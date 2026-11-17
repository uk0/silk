package prop

// builtinPropertyCategory uses exact persisted IDs, not translated labels or
// arbitrary substrings. Older widgets expose Chinese IDs, newer SDK widgets
// expose English IDs; neither may be renamed just to organize the inspector.
// Unknown extension IDs retain categoryOfPropID's compatibility heuristics.
// id is already case-folded by the classifier.
func builtinPropertyCategory(id string) string {
	switch id {
	case "控件名称", "控件类型", "类型", "object name", "widget type", "type":
		return "general"
	case "x 位置 (mm)", "y 位置 (mm)", "宽度 (mm)", "高度 (mm)",
		"x position (mm)", "y position (mm)", "width (mm)", "height (mm)",
		"上边距", "下边距", "左边距", "右边距", "内边距", "间距",
		"top margin", "bottom margin", "left margin", "right margin", "spacing",
		"布局行", "布局列", "跨行数", "跨列数", "row", "column", "rowspan", "columnspan",
		"行高", "标签宽度", "水平间距", "行间距", "最小尺寸", "弹性", "填满可用空间",
		"rowheight", "tablewidth", "daywidth", "大小", "分隔条大小", "水平方向", "垂直", "竖直", "对齐":
		return "layout"
	case "标题", "副标题", "文本", "内容", "图标", "颜色", "可见", "title", "subtitle",
		"图标大小", "圆角", "阴影", "形状", "单位", "后缀", "小数位", "格式", "format",
		"占位文本", "占位符", "新文本", "旧文本", "消息", "链接", "按钮", "label",
		"显示图例", "显示图标", "显示复选框", "显示数值", "显示文本", "显示标签",
		"显示百分比", "显示秒", "显示网格", "仅圆点", "激活图标", "根缩进",
		"打开颜色", "关闭颜色", "流动颜色", "熄灭颜色", "缩放模式", "自动缩放",
		"model source", "camera distance", "camera pitch", "camera yaw", "zoom", "show bytes", "r", "g", "b":
		return "appearance"
	case "checkable", "checked", "可用", "只读", "锁定", "选中", "选中行", "selectedindex",
		"值", "数值", "最大值", "最小值", "数量", "最大数量", "点数", "步长", "液位", "温度",
		"打开", "运行", "运行中", "激活", "点亮", "故障", "忙碌", "闪烁", "有流量", "报警阈值",
		"年", "月", "时", "分", "秒", "year", "month", "day", "多展开", "可关闭",
		"换行", "保留内容焦点", "保留上限", "maxstars", "max_visible_items",
		"当前页", "总页数", "分页步长", "相邻页数", "边界页数", "page size",
		"打印机", "打印文档", "份数", "任务标题", "items", "tasksjson", "project json",
		"device library json", "editing", "cascadedependencies", "gpu加速", "render backend":
		return "behavior"
	case TagBindingID, "url", "http method", "request body", "headers json", "json pointer",
		"result type", "timeout ms", "max response kb", "root pointer", "json text",
		"转换规则 json", "颜色规则 json", "协议", "地址", "端口", "周期ms", "点表",
		"从站号", "机架", "插槽", "设备类型", "设备编号":
		return "binding"
	}
	return ""
}

// Category keys are stable and also searchable in either built-in language,
// independently of the active locale. The row's explicit custom label still
// participates through propMatchesFilter; it never changes classification.
var categorySearchLabels = map[string]string{
	"layout": "布局", "appearance": "外观", "behavior": "行为",
	"binding": "数据绑定", "events": "事件", "general": "常规",
}

func inspectorMatchesFilter(label, id, category, filter string) bool {
	if propMatchesFilter(label, id, filter) {
		return true
	}
	if alias, ok := legacyPropertyLabels[id]; ok && propMatchesFilter(alias, "", filter) {
		return true
	}
	if alias, ok := canonicalPropertyLabels[id]; ok && propMatchesFilter(alias, "", filter) {
		return true
	}
	return propMatchesFilter(categoryNames[category], category, filter) ||
		propMatchesFilter(categorySearchLabels[category], "", filter)
}
