package gui

type DndAction int

const (
	// 忽略拖放动作
	DndIgnore DndAction = 0
	// 复制
	DndCopy DndAction = 1
	// 移动
	DndMove DndAction = 2
	// 链接
	DndLink DndAction = 4
)

//type IMimeData interface {

//	Formats() (formats []string, err error)
//	Data(format string) (data interface{}, err error)
//	SetData(data interface{}) (format string, err error)
//	Clear() error
//}

// 拖放事件的上下文参数
type IDndContext interface {
	// 可能的动作
	PosibleActions() DndAction
	// 当前动作
	Action() DndAction
	// 设置当前动作
	SetAction(act DndAction)
	// 来源
	// 来自程序内部时, 一般是某个控件
	// 来自程序外部时为nil
	From() interface{}
	// 数据的格式
	Formats() (formats []string)
	// 检测是否有指定格式的数据
	HasFormat(format string) bool
	// 获取指定格式的数据
	Data(format string) (data interface{})
}

type IOnDrop interface {
	OnDragEnter(x, y float64, dnd IDndContext)
	OnDragMove(x, y float64, dnd IDndContext)
	OnDrop(x, y float64, dnd IDndContext)
}

type IOnDragLeave interface {
	OnDragLeave()
}
