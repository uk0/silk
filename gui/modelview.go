package gui

import (
	"github.com/uk0/silk/core"
)

type ItemFlags int

const (
	NoItemFlags         ItemFlags = 0  //	It does not have any properties set.
	ItemIsSelectable    ItemFlags = 1  //It can be selected.
	ItemIsEditable      ItemFlags = 2  //It can be edited.
	ItemIsDragEnabled   ItemFlags = 4  //It can be dragged.
	ItemIsDropEnabled   ItemFlags = 8  //It can be used as a drop target.
	ItemIsUserCheckable ItemFlags = 16 //It can be checked or unchecked by the user.
	ItemIsEnabled       ItemFlags = 32 //The user can interact with the item.
	ItemIsTristate      ItemFlags = 64 //The item is checkable with three separate states.
)

type ItemDataRole int

const (
	//The general purpose roles (and the associated types) are:
	DisplayRole    ItemDataRole = 0  //The key data to be rendered in the form of text. (string)
	DecorationRole ItemDataRole = 1  //The data to be rendered as a decoration in the form of an icon. (QColor, QIcon or QPixmap)
	EditRole       ItemDataRole = 2  //The data in a form suitable for editing in an editor. (string)
	ToolTipRole    ItemDataRole = 3  //The data displayed in the item's tooltip. (string)
	StatusTipRole  ItemDataRole = 4  //The data displayed in the status bar. (string)
	WhatsThisRole  ItemDataRole = 5  //The data displayed for the item in "What's This?" mode. (string)
	SizeHintRole   ItemDataRole = 13 //The size hint for the item that will be supplied to views. (QSize)

	//Roles describing appearance and meta data (with associated types):
	FontRole             ItemDataRole = 6  //The font used for items rendered with the default delegate. (QFont)
	TextAlignmentRole    ItemDataRole = 7  //The alignment of the text for items rendered with the default delegate. (AlignmentFlag)
	BackgroundRole       ItemDataRole = 8  //The background brush used for items rendered with the default delegate. (QBrush)
	ForegroundRole       ItemDataRole = 9  //The foreground brush (text color, typically) used for items rendered with the default delegate. (QBrush)
	CheckStateRole       ItemDataRole = 10 //This role is used to obtain the checked state of an item. (CheckState)
	InitialSortOrderRole ItemDataRole = 14 //This role is used to obtain the initial sort order of a header view section. (SortOrder). This role was introduced in Qt 4.8.

	//Accessibility roles (with associated types):
	AccessibleTextRole        ItemDataRole = 11 //The text to be used by accessibility extensions and plugins, such as screen readers. (string)
	AccessibleDescriptionRole ItemDataRole = 12 //A description of the item for accessibility purposes. (string)

	//User roles:
	UserRole ItemDataRole = 32 //The first role that can be used for application-specific purposes.
)

// 模型-视图机制的数据索引
// 一个索引对应表格中的一个单元格
type ModelIndex struct {
	Row   int
	Col   int
	Param interface{}
	Model IGuiModel
}

func (v ModelIndex) Parent() ModelIndex {
	if v.Model == nil {
		return ModelIndex{}
	}
	return v.Model.Parent(v)
}

func (v ModelIndex) Child(row, col int) ModelIndex {
	if v.Model == nil {
		return ModelIndex{}
	}
	return v.Model.Index(row, col, v)
}

func (v ModelIndex) Sibling(row, col int) ModelIndex {
	if v.Model == nil {
		return ModelIndex{}
	}
	return v.Model.Index(row, col, v.Parent())
}
func (v ModelIndex) SameRow(col int) ModelIndex {
	if v.Model == nil {
		return ModelIndex{}
	}
	return v.Model.Index(v.Row, col, v.Parent())
}
func (v ModelIndex) SameCol(row int) ModelIndex {
	if v.Model == nil {
		return ModelIndex{}
	}
	return v.Model.Index(row, v.Col, v.Parent())
}

func (v ModelIndex) Flags() ItemFlags {
	if v.Model == nil {
		return 0
	}
	return v.Model.Flags(v)
}

func (v ModelIndex) IsNil() bool {
	return v == ModelIndex{}
}

////////////////////////////////////////////
type _IItemModel interface {
	bindItemView(p *GuiView)
	unbindItemView(p *GuiView)
}

////////////////////////////////////////////
// 模型-视图机制的模型, 此为抽象基类
type IGuiModel interface {
	Index(row, col int, parent ModelIndex) ModelIndex
	Data(idx ModelIndex, role ItemDataRole) interface{}
	HeaderData(section int, vertical bool, role ItemDataRole) interface{}
	Parent(idx ModelIndex) ModelIndex
	RowCount(parent ModelIndex) int
	ColCount() int // 列数必须统一, 和parent无关
	Flags(idx ModelIndex) ItemFlags
	HasChildren(mi ModelIndex) bool
}

/////////////////////////////////////////////
// 模型-视图机制的视图, 此为抽象基类
type GuiView struct {
	ScrollArea
	model IGuiModel

	cbContextMenu func(w IWidget, x, y float64)
}

func (this *GuiView) Init(iw IWidget) {
	this.ScrollArea.Init(iw)
}

func (this *GuiView) bindItemModel(m IGuiModel) {
	this.unbindItemModel()
	this.model = m
	this.model.(_IItemModel).bindItemView(this)

}

func (this *GuiView) unbindItemModel() {
	if this.model == nil {
		return
	}
	this.model.(_IItemModel).unbindItemView(this)
}

func (this *GuiView) Close() {
	this.unbindItemModel()
}

func (this *GuiView) beginReset() {
	this.Self().(interface {
		OnBeginReset()
	}).OnBeginReset()

}

func (this *GuiView) endReset() {
	this.Self().(interface {
		OnEndReset()
	}).OnEndReset()
}

func (this *GuiView) OnRightUp(x, y float64) {
	core.Debug("(this *GuiView) OnRightUp(x, y float64)")
	if this.cbContextMenu != nil {
		this.cbContextMenu(this.Self(), x, y)
	}
}

func (this *GuiView) ContextMenuCallback() func(w IWidget, x, y float64) {
	return this.cbContextMenu
}

func (this *GuiView) SetContextMenuCallback(fn func(w IWidget, x, y float64)) {
	this.cbContextMenu = fn
}

func (this *GuiView) OnBeginReset() {
}

func (this *GuiView) OnEndReset() {
}

//////////////////////////////////////////////
type GuiModel struct {
	self  IGuiModel
	views map[*GuiView]int
}

func (this *GuiModel) Init(self IGuiModel) {
	this.self = self
	this.views = make(map[*GuiView]int)
}

func (this *GuiModel) HasChildren(mi ModelIndex) bool {
	return this.self.RowCount(mi) > 0
}

func (this *GuiModel) bindItemView(p *GuiView) {
	this.views[p] = 1
}

func (this *GuiModel) unbindItemView(p *GuiView) {
	delete(this.views, p)
}

func (this *GuiModel) BeginReset() {
	// The range expression is evaluated once before beginning
	// the loop, with one exception. If the range expression is
	// an array or a pointer to an array and only the first
	// iteration value is present, only the range expression's
	// length is evaluated; if that length is constant by definition,
	//  the range expression itself will not be evaluated.
	for view, _ := range this.views {
		view.beginReset()
	}
}

func (this *GuiModel) EndReset() {
	for view, _ := range this.views {
		view.endReset()
	}
}
