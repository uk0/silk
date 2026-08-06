package ged

import (
	"github.com/uk0/silk/gui"
)

// generatedView is the read-only CodeEditor the 生成代码 tab shows.
//
// It has to stay read-only: the buffer is replaced wholesale by the next
// regeneration, so anything typed here is lost without a trace, and the file it
// mirrors carries a "DO NOT EDIT." banner. gui.CodeEditor has no read-only
// mode, so the two input entry points are overridden here instead.
type generatedView struct {
	gui.CodeEditor
}

func newGeneratedView() *generatedView {
	p := new(generatedView)
	p.Init(p)
	return p
}

// OnTextInput drops typed characters.
func (this *generatedView) OnTextInput(string) {}

// OnKeyDown forwards only keys that read the buffer. An allowlist rather than a
// blocklist: CodeEditor's key map is long and grows, and a new editing shortcut
// must not become a hole in this view by default.
func (this *generatedView) OnKeyDown(key int, repeat bool) {
	switch key {
	case gui.KeyUp, gui.KeyDown, gui.KeyLeft, gui.KeyRight,
		gui.KeyHome, gui.KeyEnd, gui.KeyPageUp, gui.KeyPageDown, gui.KeyEsc:
	case 'C', 'c', 'A', 'a', 'F', 'f':
		// Copy, select-all, find — modifier held only; the bare letters type.
		if !gui.IsKeyDown(gui.KeyCtrl) {
			return
		}
	default:
		return
	}
	this.CodeEditor.OnKeyDown(key, repeat)
}

// OnRightDown suppresses the editor's context menu, which offers 剪切/粘贴.
func (this *generatedView) OnRightDown(x, y float64) {}
