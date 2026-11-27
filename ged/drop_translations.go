package ged

import "github.com/uk0/silk/i18n"

const (
	dropContainerTooSmall = "Container is too small; resize it before dropping"
	dropInsertSlot        = "%s → %s · Insert at position %d"
	dropAppendLayout      = "%s → %s · Add to layout"
	dropReorderSlot       = "Layout · Move to position %d"
	reorderLayoutCommand  = "Reorder layout"
	dropInsertCell        = "%s → %s · Insert at row %d, column %d"
	dropAppendCell        = "%s → %s · Add row %d, column %d"
	dropMoveCell          = "Layout · Move to row %d, column %d"
	dropSwapCell          = "Layout · Swap with row %d, column %d"
	dropMoveCellCommand   = "Move layout cell"
	dropCellTaken         = "The layout cell is occupied"
	dropCellGap           = "Drop inside a visible layout cell"
	dropLayoutLocked      = "The layout or control is locked"
	dropTransferCommand   = "Move control between layouts"
	dropTransferSingle    = "Select one control to move between layouts"
	dropTransferOrder     = "Move to %s · Position %d"
	dropTransferCell      = "Move to %s · Row %d, column %d"
	dropTransferSpan      = "This layout cannot preserve the control's span"
	dropTransferUnsafe    = "Moving here would displace a locked control or clip a control"

	dropCellClipped        = "The control would be clipped; enlarge the layout"
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
		dropInsertCell:                   "%s → %s（插入第 %d 行，第 %d 列）",
		dropAppendCell:                   "%s → %s（新增第 %d 行，第 %d 列）",
		dropMoveCell:                     "自动布局 · 移到第 %d 行，第 %d 列",
		dropSwapCell:                     "自动布局 · 与第 %d 行，第 %d 列交换",
		dropMoveCellCommand:              "移动布局单元格",
		dropCellTaken:                    "此单元格已被占用",
		dropCellGap:                      "请移入可见的布局单元格",
		dropLayoutLocked:                 "布局或控件已锁定",
		dropCellClipped:                  "控件会被裁掉，请先扩大布局",
		dropTransferCommand:              "跨布局移动控件",
		dropTransferSingle:               "请选中一个需要跨布局移动的控件",
		dropTransferOrder:                "移入 %s · 第 %d 位",
		dropTransferCell:                 "移入 %s · 第 %d 行，第 %d 列",
		dropTransferSpan:                 "目标布局无法保留此控件的跨度",
		dropTransferUnsafe:               "移动会改变锁定控件的位置或使控件被裁切",
		managedGeometryMessage:           "自动布局管理位置；解除布局后可自由移动或调整尺寸",
	})
}
