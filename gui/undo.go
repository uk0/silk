package gui

type ICommand interface {
	Redo()
	Undo()
	Text() string
}

type CompoundCommand struct {
	cmds []ICommand
	text string
}

func NewCompoundCommand() *CompoundCommand {
	return new(CompoundCommand)
}

func (this *CompoundCommand) Undo() {
	for i := len(this.cmds) - 1; i >= 0; i-- {
		this.cmds[i].Undo()
	}
}

func (this *CompoundCommand) Redo() {
	for _, v := range this.cmds {
		v.Redo()
	}
}

func (this *CompoundCommand) Text() string {
	return this.text
}

func (this *CompoundCommand) SetText(s string) {
	this.text = s
}

func (this *CompoundCommand) Append(a ICommand) {
	this.cmds = append(this.cmds, a)
}

// 6 <nil>  <- stack top, count = 5
// 5  Cmd5
// 4  Cmd4
// 3 [Cmd3] <- current index, redo will excute this
// 2  Cmd2  <- previous index, undo vill excute this
// 1  Cmd1
// 0  Cmd0  <- stack bottom

// 支持撤销/恢复的命令堆栈
type IUndoStack interface {
	// 命令压栈并执行
	Push(cmd ICommand)

	// 能否撤销
	CanUndo() bool

	// 撤销
	Undo()

	// 描述当前撤销操作的文本, 例如"撤销某操作"
	UndoText() string

	// 能否恢复
	CanRedo() bool

	// 恢复
	Redo()

	// 描述当前重做操作的文本, 例如"恢复某操作"
	RedoText() string

	// 堆栈里的全部命令的数目, 含撤销和恢复
	Count() int

	// 当前命令的索引, 指向下一步Push或Redo的命令
	Current() int

	// 获取指定位置的命令
	// 此方法只供查询命令的信息, 请不要在外部执行获取到的命令, 否则将发生混乱
	Command(index int) ICommand

	// 把当前位置设置为"清洁"状态, 表示文档在当前位置已保存
	SetClean()

	// 判断当前位置是否"清洁"状态
	IsClean() bool

	// Undo方法对应的Action
	UndoAction() IAction

	// Redo方法对应的Action
	RedoAction() IAction

	// 清除所有命令, 并重置Clean和Current位置
	Clear()
}

// 支持撤销恢复的命令堆栈
type UndoStack struct {
	name       string
	cmds       []ICommand
	current    int
	clean      int
	undoAction *Action
	redoAction *Action
}

func NewUndoStack(name string) *UndoStack {
	p := new(UndoStack)
	p.name = name
	p.undoAction = NewAction()
	p.redoAction = NewAction()
	p.undoAction.SetIcon(LoadIcon("edit-undo"))
	p.redoAction.SetIcon(LoadIcon("edit-redo"))
	if name == "" {
		p.undoAction.SetObjName("edit-undo")
		p.redoAction.SetObjName("edit-redo")

	} else {
		p.undoAction.SetObjName(name + "-undo")
		p.redoAction.SetObjName(name + "-redo")
	}
	p.undoAction.BindFunc0(p.Undo)
	p.redoAction.BindFunc0(p.Redo)
	p.cmds = []ICommand{}
	p.syncActions()
	return p
}

func (this *UndoStack) Push(cmd ICommand) {
	if cmd == nil {
		return
	}
	// try merge
	if this.current > 0 && this.clean != this.current {
		prev := this.cmds[this.current-1]
		if im, ok := prev.(interface {
			MergeWidth(ICommand) bool
		}); ok {
			if im.MergeWidth(cmd) {
				cmd.Redo()
				this.syncActions()
				return
			}
		}
	}
	this.cmds = append(this.cmds[:this.current], cmd)
	this.Redo()
}

func (this *UndoStack) CanUndo() bool {
	return this.current > 0
}

func (this *UndoStack) Undo() {
	if !this.CanUndo() {
		return
	}
	this.current--
	this.cmds[this.current].Undo()
	this.syncActions()
	if this.clean == this.current || this.clean == this.current+1 {
		this.emitCheanChanged()
	}
}

func (this *UndoStack) UndoText() string {
	if this.current > 0 {
		return "Undo " + this.cmds[this.current-1].Text()
	}
	return "Undo"
}

func (this *UndoStack) CanRedo() bool {
	return this.current < this.Count()
}

func (this *UndoStack) Redo() {
	if !this.CanRedo() {
		return
	}
	this.cmds[this.current].Redo()
	this.current++
	this.syncActions()
	if this.clean == this.current || this.clean == this.current-1 {
		this.emitCheanChanged()
	}

}

func (this *UndoStack) RedoText() string {
	if this.current < this.Count() {
		return "Redo " + this.cmds[this.current].Text()
	}
	return "Redo"
}

func (this *UndoStack) Count() int {
	return len(this.cmds)
}

func (this *UndoStack) Current() int {
	return this.current
}

func (this *UndoStack) Command(index int) ICommand {
	return this.cmds[index]
}

func (this *UndoStack) SetClean() {
	if this.clean != this.current {
		this.clean = this.current
		this.emitCheanChanged()
	}
}

func (this *UndoStack) IsClean() bool {
	return this.current == this.clean
}

func (this *UndoStack) UndoAction() IAction {
	return this.undoAction
}

func (this *UndoStack) RedoAction() IAction {
	return this.redoAction
}

func (this *UndoStack) emitCheanChanged() {

}

func (this *UndoStack) Clear() {
	if this.IsClean() {
		this.clean = 0
	} else {
		this.clean = -1
	}
	this.current = 0
	this.undoAction.SetEnabled(false)
	this.redoAction.SetEnabled(false)
	this.cmds = []ICommand{}

}

func (this *UndoStack) syncActions() {
	this.undoAction.SetEnabled(this.CanUndo())
	this.undoAction.SetText(this.UndoText())
	this.redoAction.SetEnabled(this.CanRedo())
	this.redoAction.SetText(this.RedoText())
}
