package gui

import (
	"path/filepath"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/paint"
)

func init() {
	core.RegisterFactory("gui.PathEdit", core.TypeOf((*PathEdit)(nil)))
}

// PathMode picks which dialog the browse button opens. The three
// values map onto the OpenFileDialog / SaveFileDialog primitives the
// framework already ships; folder-pick reuses OpenFileDialog and
// keeps the parent directory of the picked file since the GLFW build
// of the framework has no native folder picker.
type PathMode int

const (
	PathFile PathMode = iota
	PathFolder
	PathSaveFile
)

// pathEditButtonWidth is the fixed width of the right-side "Browse..."
// button region. Wide enough for "..." plus padding at the default
// theme font size; the text input takes whatever remains to the left.
const pathEditButtonWidth = 32.0

// pathEditButtonLabel is the glyph drawn on the browse button. A
// short "..." sticks to Qt Designer's QFileEdit convention and keeps
// the cell narrow enough that the text field still gets most of the
// width on a 200px-wide control.
const pathEditButtonLabel = "..."

// PathEdit is a single-line text input paired with a fixed-width
// browse button on the right. Clicking the button pops the framework
// file/folder dialog (per Mode) and writes the picked path back into
// the field; typing into the field works the same as a plain Edit.
//
// Usage:
//
//	pe := gui.NewPathEdit()
//	pe.SetMode(gui.PathFolder)
//	pe.SigPathChanged(func(p string) { config.OutputDir = p })
type PathEdit struct {
	Widget

	text        string
	placeholder string
	mode        PathMode
	readonly    bool

	// openFn opens the file/folder dialog and returns (path, ok).
	// Defaults to a real dialog driver; tests override it to a stub
	// because the real native dialog requires a window/event loop
	// and would block headless test runs.
	openFn func(mode PathMode) (string, bool)

	cbPathChanged func(string)
}

// NewPathEdit creates an empty path picker in PathFile mode wired to
// the framework's real file dialog. Tests that need to drive the
// browse-button decision without spawning the native dialog assign
// their own openFn via the (unexported) setter or by direct write —
// see pathedit_test.go.
func NewPathEdit() *PathEdit {
	p := new(PathEdit)
	p.Init(p)
	p.mode = PathFile
	p.openFn = defaultPathEditOpenFn
	return p
}

// Text returns the current path string.
func (this *PathEdit) Text() string { return this.text }

// SetText writes the path string. Fires SigPathChanged only when the
// value actually changes, so a redundant SetText(current) is a cheap
// no-op (matches Pagination.SetCurrentPage's semantics).
func (this *PathEdit) SetText(s string) {
	if s == this.text {
		return
	}
	this.text = s
	if this.cbPathChanged != nil {
		this.cbPathChanged(s)
	}
	this.Self().Update()
}

// Mode returns the configured PathMode.
func (this *PathEdit) Mode() PathMode { return this.mode }

// SetMode switches the dialog flavor the browse button will pop.
// Defaults to PathFile.
func (this *PathEdit) SetMode(m PathMode) {
	this.mode = m
}

// SetPlaceholder sets the grey hint string drawn when Text() is empty.
func (this *PathEdit) SetPlaceholder(s string) {
	this.placeholder = s
	this.Self().Update()
}

// Placeholder returns the current placeholder string.
func (this *PathEdit) Placeholder() string { return this.placeholder }

// SigPathChanged registers the path-change callback. Fired by
// SetText (whether triggered by the dialog or by a programmatic
// caller) only on a real change.
func (this *PathEdit) SigPathChanged(fn func(string)) {
	this.cbPathChanged = fn
}

// IsReadOnly reports whether the field is read-only. A read-only
// PathEdit still pops the dialog on a button click — read-only here
// means the user can't type characters into the field, mirroring
// QLineEdit's setReadOnly.
func (this *PathEdit) IsReadOnly() bool { return this.readonly }

// SetReadOnly toggles read-only typing. The browse button stays live
// because picking via dialog is the intended workflow when the user
// shouldn't free-form-type a path.
func (this *PathEdit) SetReadOnly(b bool) {
	this.readonly = b
	this.Self().Update()
}

// SetOpenFn replaces the dialog-opening function. Used by tests to
// stub out the real native dialog (which needs a window + event
// loop) and observe whether the browse-button decision was reached.
// Passing nil restores the default real-dialog driver.
func (this *PathEdit) SetOpenFn(fn func(mode PathMode) (string, bool)) {
	if fn == nil {
		fn = defaultPathEditOpenFn
	}
	this.openFn = fn
}

// buttonHitTest reports whether x falls inside the right-side
// browse-button region, given the widget's total width w and the
// button-region width btnW. Pulled out as a free function so the
// boundary logic is unit-testable without standing up GL state.
//
// The button region is [w-btnW, w). x < 0 or x >= w returns false.
func buttonHitTest(x, w, btnW float64) bool {
	if btnW <= 0 || w <= 0 {
		return false
	}
	if x < 0 || x >= w {
		return false
	}
	return x >= w-btnW
}

// resolvePickedPath maps a raw dialog-returned path to the value
// that should land in the text field, according to the current
// mode. For PathFolder the GLFW build returns a file path because
// the framework has no folder picker, so we walk up to the parent
// directory; PathFile / PathSaveFile pass the path through.
//
// Pure helper so the (small) parent-of-file fallback for PathFolder
// is unit-testable.
func resolvePickedPath(mode PathMode, raw string) string {
	if raw == "" {
		return ""
	}
	if mode == PathFolder {
		return filepath.Dir(raw)
	}
	return raw
}

// defaultPathEditOpenFn dispatches to the framework's native file
// dialog. PathFolder reuses OpenFileDialog because the GLFW build
// has no folder picker; the picked file's parent directory becomes
// the chosen folder.
func defaultPathEditOpenFn(mode PathMode) (string, bool) {
	var raw string
	if mode == PathSaveFile {
		raw = SaveFileDialog()
	} else {
		raw = OpenFileDialog()
	}
	if raw == "" {
		return "", false
	}
	return resolvePickedPath(mode, raw), true
}

// OnLeftDown routes the click: a hit in the button region opens the
// dialog and writes any picked path back via SetText (which fires
// SigPathChanged on a real change). A hit in the text region just
// grabs focus so the user can type.
func (this *PathEdit) OnLeftDown(x, y float64) {
	if buttonHitTest(x, this.w, pathEditButtonWidth) {
		this.SetFocus()
		if this.openFn == nil {
			return
		}
		path, ok := this.openFn(this.mode)
		if !ok || path == "" {
			return
		}
		this.SetText(path)
		return
	}
	this.SetFocus()
}

// OnTextInput accepts typed characters and appends them to the field,
// matching QLineEdit's behaviour. No-op when read-only.
func (this *PathEdit) OnTextInput(s string) {
	if this.readonly {
		return
	}
	if s == "" {
		return
	}
	this.SetText(this.text + s)
}

// OnKeyDown handles Backspace; other keys fall through. Kept minimal
// because the dialog is the primary input path — anyone needing full
// editing should compose a real gui.Edit beside this widget.
func (this *PathEdit) OnKeyDown(key int, repeat bool) {
	if this.readonly {
		return
	}
	if key == KeyBackSpace {
		if len(this.text) == 0 {
			return
		}
		runes := []rune(this.text)
		this.SetText(string(runes[:len(runes)-1]))
	}
}

// OnMouseEnter / OnMouseLeave keep the hover paint in sync.
func (this *PathEdit) OnMouseEnter() { this.Self().Update() }
func (this *PathEdit) OnMouseLeave() { this.Self().Update() }

// Cursor shows an I-beam over the text region and the default arrow
// over the button. Matches Edit's iBeam over its body.
func (this *PathEdit) Cursor() *Cursor {
	return cursorIBeam
}

// Draw paints the text field on the left and the "..." browse button
// on the right. Uses the theme's edit frame so the input matches a
// stock gui.Edit; the button shares Theme().FormDarkColor for its
// background to read as a flat affordance against the white field.
func (this *PathEdit) Draw(g paint.Painter) {
	g.Save()
	defer g.Restore()

	t := Theme()
	iw := this.Self()
	w, h := this.w, this.h
	btnW := pathEditButtonWidth
	if btnW > w {
		btnW = w
	}
	textW := w - btnW

	// Left: text-field background + bordered frame, like Edit.
	g.Rectangle(0, 0, textW, h)
	if this.readonly {
		g.SetBrush1(t.FormColor)
	} else {
		g.SetBrush1(t.ViewBGColor)
	}
	g.Fill()
	t.DrawEditFrame(g, 0, 0, textW, h, iw.HasFocus(), iw.IsHover(), this.readonly)

	// Text or placeholder, padded into the frame like Edit's padding.
	m := t.EditPadding
	g.SetFont(t.Font)
	fe := t.Font.FontExtents()
	textY := (h + fe.Ascent - fe.Descent) / 2
	if this.text != "" {
		g.SetBrush1(t.TextColor)
		g.DrawText1(m.L, textY, this.text)
	} else if this.placeholder != "" {
		g.SetBrush1(t.FormDarkColor)
		g.DrawText1(m.L, textY, this.placeholder)
	}

	// Right: the browse button. Flat fill + frame so it reads as a
	// clickable affordance without overpowering the text field.
	bx := w - btnW
	g.Rectangle(bx, 0, btnW, h)
	g.SetBrush1(t.FormDarkColor)
	g.Fill()
	g.SetPen1(t.BorderColor, 1)
	g.Rectangle(bx, 0, btnW, h)
	g.Stroke()

	// Centered "..." label on the button.
	ext := t.Font.TextExtents(pathEditButtonLabel)
	tx := bx + (btnW-ext.Width)/2 - ext.XBearing
	ty := (h + fe.Ascent - fe.Descent) / 2
	g.SetBrush1(t.TextColor)
	g.DrawText1(tx, ty, pathEditButtonLabel)
}

// SizeHints reports a sensible default 200x28; the widget grows
// horizontally so it tracks layout width and the text region absorbs
// the slack to the left of the fixed-width button.
func (this *PathEdit) SizeHints() SizeHints {
	return SizeHints{
		Width:  200,
		Height: 28,
		Policy: GrowHorizontal,
	}
}

// EnumProperties exposes the path/placeholder/readonly knobs to the
// designer's property sheet. Mode is intentionally omitted — picking
// "file vs folder" is wiring code, not visual styling.
func (this *PathEdit) EnumProperties(list core.IPropertyList) {
	list.AddProperty("路径", this.Text, this.SetText)
	list.AddProperty("占位符", this.Placeholder, this.SetPlaceholder)
	list.AddProperty("只读", this.IsReadOnly, this.SetReadOnly)
}
