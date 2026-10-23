package ged

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"reflect"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func eventDefinition(factory, event string) (widgetEvent, bool) {
	for _, def := range widgetEvents[factory] {
		if def.name == event {
			return def, true
		}
	}
	return widgetEvent{}, false
}

func defaultHandlerName(w *FakeWidget, event string) string {
	var b strings.Builder
	for _, r := range w.WidgetName() {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	name := b.String()
	if name == "" {
		name = "widget"
	}
	return "on" + capitalize(name) + strings.TrimPrefix(event, "On")
}

// SetEventBinding is the designer mutation API. Property editors already wrap
// their setters in commands; context menus must use this API instead of calling
// the raw model setter and silently bypassing dirty/undo tracking.
func (s *GedScene) SetEventBinding(w *FakeWidget, event, handler string) error {
	if w == nil || !s.containsWidget(w) {
		return fmt.Errorf("控件不属于当前设计")
	}
	handler = strings.TrimSpace(handler)
	if _, ok := eventDefinition(w.WidgetFactoryName(), event); !ok && handler != "" {
		return fmt.Errorf("不支持的控件事件: %s", event)
	}
	if handler != "" && (!token.IsIdentifier(handler) || handler == "_") {
		return fmt.Errorf("处理函数名称必须是有效的 Go 标识符")
	}
	if err := s.CommitPendingEdits(); err != nil {
		return err
	}
	before := w.EventHandlers()[event]
	if before == handler {
		return nil
	}
	s.PushCommand(&eventHandlerCommand{widget: w, event: event, beforeName: before, afterName: handler, beforeCode: w.GetCode(), afterCode: w.GetCode()})
	return nil
}

type eventHandlerCommand struct {
	widget                                              *FakeWidget
	event, beforeName, afterName, beforeCode, afterCode string
}

func (c *eventHandlerCommand) apply(name, code string) {
	if name == "" {
		c.widget.RemoveEventHandler(c.event)
	} else {
		c.widget.SetEventHandler(c.event, name)
	}
	c.widget.SetCode(code)
}
func (c *eventHandlerCommand) Undo()        { c.apply(c.beforeName, c.beforeCode) }
func (c *eventHandlerCommand) Redo()        { c.apply(c.afterName, c.afterCode) }
func (c *eventHandlerCommand) Text() string { return "编辑事件绑定" }

func findHandler(code, name string) (*ast.FuncDecl, int, int, error) {
	fset, file, err := parseHandlerFile(code)
	if err != nil {
		return nil, 0, 0, err
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name {
			continue
		}
		offset := fset.Position(fn.Name.Pos()).Offset - len(handlerPackagePrefix)
		prefix := code[:offset]
		line := strings.Count(prefix, "\n")
		start := strings.LastIndex(prefix, "\n") + 1
		return fn, line, utf8.RuneCountInString(prefix[start:]), nil
	}
	return nil, 0, 0, nil
}

func parameterTypes(fn *ast.FuncType) []string {
	var out []string
	for _, field := range fn.Params.List {
		var buf bytes.Buffer
		_ = format.Node(&buf, token.NewFileSet(), field.Type)
		for range max(1, len(field.Names)) {
			out = append(out, buf.String())
		}
	}
	return out
}

// OpenEventHandler creates or locates an embedded design handler. It does not
// pretend to edit a user-owned generated companion file: that file is separate.
// Creation uses codegen's own event signatures and a single undoable command.
func (p *CodePanel) OpenEventHandler(w *FakeWidget, event string) error {
	s := p.scene()
	if s == nil || w == nil || !s.containsWidget(w) {
		return fmt.Errorf("控件不属于当前设计")
	}
	def, ok := eventDefinition(w.WidgetFactoryName(), event)
	if !ok {
		return fmt.Errorf("不支持的控件事件: %s", event)
	}
	if err := s.CommitPendingEdits(); err != nil {
		return err
	}
	name := w.EventHandlers()[event]
	explicit := name != ""
	if !explicit && event == defaultEventForFactory(w.WidgetFactoryName()) {
		name = extractHandlerName(w.GetCode())
	}
	used := map[string]bool{}
	walkFakeWidgets(s.Children(), func(f *FakeWidget) {
		_, file, err := parseHandlerFile(f.GetCode())
		if err != nil {
			return
		}
		for _, decl := range file.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok {
				used[fn.Name.Name] = true
			}
		}
		for _, binding := range f.EventHandlers() {
			used[binding] = true
		}
	})
	if name == "" {
		base := defaultHandlerName(w, event)
		name = base
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("%s%d", base, i)
		}
	}
	if !token.IsIdentifier(name) || name == "_" {
		return fmt.Errorf("无效的处理函数名称: %s", name)
	}
	owner := w
	var found *ast.FuncDecl
	var locateErr error
	// A shared binding points to its existing source rather than seeding a
	// second definition on another widget. Ambiguous declarations are rejected.
	walkFakeWidgets(s.Children(), func(f *FakeWidget) {
		fn, _, _, err := findHandler(f.GetCode(), name)
		if err != nil {
			if f == w {
				locateErr = err
			}
			return
		}
		if fn != nil {
			if found != nil {
				locateErr = fmt.Errorf("重复的事件处理函数: %s", name)
			}
			found = fn
			owner = f
		}
	})
	if locateErr != nil {
		return locateErr
	}
	if found != nil {
		expected, _, _, _ := findHandler("func expected("+def.params+") {}", "expected")
		if !reflect.DeepEqual(parameterTypes(found.Type), parameterTypes(expected.Type)) || found.Type.Results != nil && len(found.Type.Results.List) > 0 || found.Type.TypeParams != nil {
			return fmt.Errorf("%s 的签名与 %s(%s) 不兼容", name, event, def.params)
		}
	}
	code := w.GetCode()
	if found == nil {
		if _, _, err := parseHandlerFile(code); err != nil {
			return err
		}
		stub := fmt.Sprintf("%s%s(%s) {\n\t// TODO: implement %s.\n}\n", handlerReceiver(defaultCodeGenOptions(s, CodeGenOptions{}).TypeName), name, def.params, event)
		if strings.TrimSpace(code) != "" {
			code = strings.TrimRight(code, "\n") + "\n\n"
		}
		code += stub
	}
	if w.EventHandlers()[event] != name || w.GetCode() != code {
		s.PushCommand(&eventHandlerCommand{widget: w, event: event, beforeName: w.EventHandlers()[event], afterName: name, beforeCode: w.GetCode(), afterCode: code})
	}
	p.SetWidget(owner)
	p.ShowTab(codeTabHandler)
	_, line, col, err := findHandler(p.editor.Text(), name)
	if err != nil {
		return err
	}
	p.editor.NavigateToLineCol(line, col)
	p.Focus()
	return nil
}

func (v *GedView) SetEventHandlerCallback(fn func(*FakeWidget, string) error) {
	v.showEventHandler = fn
}

func (v *GedView) RequestEventHandler(w *FakeWidget, event string) bool {
	if w == nil || v.GedScene() == nil || !v.GedScene().containsWidget(w) {
		return false
	}
	var err error
	if v.showEventHandler != nil {
		err = v.showEventHandler(w, event)
	} else if v.eventCodePanel != nil {
		v.requestCodePanel()
		err = v.eventCodePanel.OpenEventHandler(w, event)
	} else {
		return false
	}
	if err != nil {
		gui.ShowToast(v, err.Error(), 4000, gui.ToastError)
		return false
	}
	return true
}

// EventHandlerMenu lists only events the SDK generator can really wire.
func (v *GedView) EventHandlerMenu(w *FakeWidget) *gui.Menu {
	menu := gui.NewPopupMenu()
	if w == nil {
		return menu
	}
	for _, event := range AvailableEvents(w.WidgetFactoryName()) {
		label := event
		if name := w.EventHandlers()[event]; name != "" {
			label += ": " + name
		}
		b := menu.AddButton1(label, paint.LoadIcon("function"))
		b.Action().SetObjName("event.handler." + event)
		enabled := v.showEventHandler != nil || v.eventCodePanel != nil
		b.Action().SetEnabled(enabled)
		b.SetEnabled(enabled)
		b.Action().BindFunc0(func() { v.RequestEventHandler(w, event) })
	}
	return menu
}
