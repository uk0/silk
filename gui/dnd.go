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

// dndDragLeave tells w the drag has left it; w may be nil.
func dndDragLeave(w IWidget) {
	if i, ok := w.(IOnDragLeave); ok {
		i.OnDragLeave()
	}
}

// dndDragOver offers the drag at (x, y) — widget's own coordinates — to widget
// and then to each of its ancestors, and returns the widget that now holds the
// drag, nil when none of them takes it. The accepted action is left in ctx.
//
// A widget that refuses the drag has to fall through to its parent: Dock
// deliberately refuses near its own border so the enclosing Frame can take the
// drag and dock the view to the frame edge, and Frame refuses outside its root
// brick. Offering the drag only to the innermost IOnDrop under the cursor makes
// edge docking impossible. Both backends share this walk instead of each
// keeping its own copy of the rule.
func dndDragOver(widget, last IWidget, x, y float64, ctx IDndContext) IWidget {
	for widget != nil {
		i, ok := widget.(IOnDrop)
		if ok && widget == last {
			ctx.SetAction(DndIgnore)
			i.OnDragMove(x, y, ctx)
			if ctx.Action() != DndIgnore {
				return widget
			}
		} else if ok {
			ctx.SetAction(DndIgnore)
			i.OnDragEnter(x, y, ctx)
			if ctx.Action() != DndIgnore {
				dndDragLeave(last)
				last = nil
				ctx.SetAction(DndIgnore)
				i.OnDragMove(x, y, ctx)
				if ctx.Action() != DndIgnore {
					return widget
				}
				dndDragLeave(widget)
			}
		}
		x += widget.X()
		y += widget.Y()
		widget = widget.Parent()
	}
	dndDragLeave(last)
	// Nobody took the drag: drop the action the last accepting target left
	// behind, or the feedback cursor keeps offering a drop over dead space.
	ctx.SetAction(DndIgnore)
	return nil
}

// dndDrop hands the drop at (x, y) — widget's own coordinates — to the first
// widget from widget up the tree that claims it by setting an action.
func dndDrop(widget IWidget, x, y float64, ctx IDndContext) {
	ctx.SetAction(DndIgnore)
	for widget != nil {
		if i, ok := widget.(IOnDrop); ok {
			i.OnDrop(x, y, ctx)
			if ctx.Action() != DndIgnore {
				return
			}
		}
		x += widget.X()
		y += widget.Y()
		widget = widget.Parent()
	}
}
