// Tab overflow demonstrates clipped-title-safe overflow, page identity,
// keyboard navigation and drag reordering using only public SDK APIs.
package main

import (
	"encoding/json"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

type tabDemo struct {
	gui.Form
	tabs         *gui.TabWidget
	help, status *gui.Label
	buttons      []*gui.Button
	narrow       bool
}
type tabState struct {
	Index       int
	Title, Text string
	Order       []string
	Narrow      bool
}

func newTabDemo() *tabDemo {
	f := new(tabDemo)
	f.Form.Init(f)
	f.SetTitle("Silk - Tab Overflow")
	f.help = gui.NewLabel("Use the chooser, Ctrl+Tab, mouse wheel or drag tabs. Page text stays with its title.")
	f.help.SetParent(f)
	f.status = gui.NewLabel("")
	f.status.SetParent(f)
	f.tabs = gui.NewTabWidget()
	f.tabs.SetParent(f)
	for _, title := range []string{"Properties", "Outline", "Bookmarks", "中文🙂 notes", "Network requests", "Very long document title"} {
		editor := gui.NewCodeEditor()
		editor.SetText("// " + title + "\n中🙂 seed\n")
		f.tabs.AddTab(editor, title, paint.LoadIcon("file"))
	}
	f.tabs.SetCurrentChangedCallback(func(interface{}, int) { f.refresh() })
	f.tabs.TabBar().SetCloseCallback(func(_ *gui.TabBar, i int) bool { f.tabs.RemoveTab(i); return true })
	add := func(text, icon string, fn func()) {
		b := gui.NewButton1(text, paint.LoadIcon(icon))
		b.SetTextVisible(true)
		b.SetParent(f)
		b.Action().BindFunc0(fn)
		f.buttons = append(f.buttons, b)
	}
	add("Previous", "navigate-back", func() { f.step(-1) })
	add("Next", "navigate-forward", func() { f.step(1) })
	add("Narrow / Wide", "toolbar-overflow", func() { f.narrow = !f.narrow; f.OnResize(); f.refresh() })
	add("Snapshot", "log", func() { data, _ := json.Marshal(f.state()); fmt.Println("SNAPSHOT " + string(data)) })
	f.refresh()
	return f
}
func (f *tabDemo) step(delta int) {
	if n := f.tabs.Count(); n > 0 {
		f.tabs.SetCurrentIndex((f.tabs.CurrentIndex() + delta + n) % n)
		if page := f.tabs.Stack().CurrentPage(); page != nil {
			page.SetFocus()
		}
	}
}
func (f *tabDemo) state() tabState {
	s := tabState{Index: f.tabs.CurrentIndex(), Narrow: f.narrow}
	for i := 0; i < f.tabs.Count(); i++ {
		s.Order = append(s.Order, f.tabs.TabTitle(i))
	}
	s.Title = f.tabs.TabTitle(s.Index)
	if e, ok := f.tabs.Stack().CurrentPage().(*gui.CodeEditor); ok {
		s.Text = e.Text()
	}
	return s
}
func (f *tabDemo) refresh() {
	f.status.SetText(fmt.Sprintf("Page %d / %d: %s", f.tabs.CurrentIndex()+1, f.tabs.Count(), f.tabs.TabTitle(f.tabs.CurrentIndex())))
}
func (f *tabDemo) OnResize() {
	if f.tabs == nil {
		return
	}
	w, h := f.Size()
	w, h = max(0, w), max(0, h)
	place := func(c gui.IWidget, x, y, cw, ch float64) {
		x, y = max(0, min(x, w)), max(0, min(y, h))
		c.SetBounds(x, y, max(0, min(cw, w-x)), max(0, min(ch, h-y)))
	}
	place(f.help, 16, 12, w-32, 32)
	width := w - 32
	if f.narrow {
		width = min(width, 185)
	}
	place(f.tabs, 16, 52, width, max(0, h-160))
	f.tabs.Layout()
	y := max(52, h-100)
	place(f.status, 16, y, w-32, 24)
	y += 28
	x := 16.0
	for _, b := range f.buttons {
		bw := max(110, b.SizeHints().Width+12)
		if x > 16 && x+bw > w-16 {
			x = 16
			y += 38
		}
		place(b, x, y, bw, 32)
		x += bw + 8
	}
}
func main() {
	defer core.Close()
	f := newTabDemo()
	f.AttachWindow(gui.WtForm)
	f.Window().SetSize(760, 430)
	f.Window().MoveToCenter()
	f.Show()
	f.tabs.Stack().CurrentPage().SetFocus()
	core.EventLoop()
}
