package ged

import "github.com/uk0/silk/i18n"

const (
	dropContainerTooSmall  = "Container is too small; resize it before dropping"
	dropInsertSlot         = "%s → %s · Insert at position %d"
	dropAppendLayout       = "%s → %s · Add to layout"
	dropReorderSlot        = "Layout · Move to position %d"
	reorderLayoutCommand   = "Reorder layout"
	managedGeometryMessage = "The layout controls this geometry. Break the layout to move or resize freely."
)

func init() { registerDropTranslations() }

func registerDropTranslations() {
	i18n.AddMany("zh-CN", map[string]string{
		"Drag a widget from the palette": "请从控件面板拖入",
		"Drop inside the form":           "请移入表单范围",
		"Form":                           "表单",
		dropContainerTooSmall:            "容器空间不足，请先调整大小",
		dropInsertSlot:                   "%s → %s（自动布局 · 插入第 %d 位）",
		dropAppendLayout:                 "%s → %s（追加到自动布局）",
		dropReorderSlot:                  "自动布局 · 移到第 %d 位",
		reorderLayoutCommand:             "调整自动布局顺序",
		managedGeometryMessage:           "自动布局管理位置；解除布局后可自由移动或调整尺寸",
	})
}
