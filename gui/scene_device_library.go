package gui

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/scene3d"
)

const MaxSceneLibraryDevices = 64
const MaxSceneLibraryBytes = 64 << 20 // aggregate TemplateStats estimate, not process RSS

var sceneDeviceFileDialog = OpenFileDialog
var sceneDeviceLoader = scene3d.LoadTemplateFile

type sceneDeviceLibrary struct {
	Widget
	editor                     *SceneDocumentEditor
	devices                    []scene3d.DeviceDefinition
	rows                       []int
	back, importButton, insert *Button
	search                     *SearchBox
	prefix                     *Edit
	list                       *ListWidget
	prefixLabel, details       *Label
	generation                 uint64
	cancel                     context.CancelFunc
	busy                       bool
	initialized                bool
	selectedID                 string
	lastError                  error
}

// SetDeviceLibrary replaces this editor's shelf atomically. Definitions are
// copied; immutable templates may be shared with plugins/other editors. No disk,
// tag or driver is activated. A nil/empty shelf is allowed. All calls are UI-owned.
func (e *SceneDocumentEditor) SetDeviceLibrary(devices []scene3d.DeviceDefinition) error {
	if e.closed {
		return async.ErrClosed
	}
	if len(devices) > MaxSceneLibraryDevices {
		return scene3d.ErrLimit
	}
	ids := make(map[string]bool, len(devices))
	bytes := 0
	for _, d := range devices {
		if !sceneDeviceText(d.ID, 256) || !sceneDeviceText(d.Name, 256) || d.Template == nil || d.Template.NodeCount() == 0 || ids[d.ID] {
			return scene3d.ErrNode
		}
		ids[d.ID] = true
		bytes += d.Template.Stats().EstimatedBytes
		if bytes > MaxSceneLibraryBytes {
			return scene3d.ErrLimit
		}
	}
	e.ensureDeviceLibrary()
	e.library.cancelImport()
	e.library.initialized = true
	e.library.devices = append([]scene3d.DeviceDefinition(nil), devices...)
	e.preview.OnDragLeave()
	e.library.filter()
	return nil
}
func sceneDeviceText(s string, limit int) bool {
	if strings.TrimSpace(s) == "" || len(s) > limit || !utf8.ValidString(s) {
		return false
	}
	return strings.IndexFunc(s, unicode.IsControl) < 0
}

// DeviceLibrary returns independent metadata; referenced templates are immutable.
func (e *SceneDocumentEditor) DeviceLibrary() []scene3d.DeviceDefinition {
	if e.closed || e.library == nil {
		return nil
	}
	return append([]scene3d.DeviceDefinition(nil), e.library.devices...)
}
func (e *SceneDocumentEditor) DeviceLibraryVisible() bool { return !e.closed && e.libraryVisible }
func (e *SceneDocumentEditor) SetDeviceLibraryVisible(visible bool) {
	if e.closed {
		return
	}
	e.ensureDeviceLibrary()
	if visible && !e.library.initialized {
		defs, err := scene3d.BuiltinDeviceTemplates()
		if err != nil {
			e.report(err)
			return
		}
		if err = e.SetDeviceLibrary(defs); err != nil {
			e.report(err)
			return
		}
	}
	e.libraryVisible = visible
	if !visible {
		e.library.cancelImport()
		e.preview.OnDragLeave()
	}
	e.tree.SetVisible(!visible)
	e.bindings.SetVisible(!visible)
	e.library.SetVisible(visible)
	e.syncActions()
	e.Layout()
	e.Update()
}
func (e *SceneDocumentEditor) ensureDeviceLibrary() {
	if e.library != nil {
		return
	}
	l := &sceneDeviceLibrary{editor: e}
	l.Init(l)
	l.SetParent(e)
	l.SetVisible(false)
	e.library = l
	button := func(text, icon string, run func()) *Button {
		b := NewButton()
		b.SetParent(l)
		b.SetText(i18n.T(text))
		b.SetTextVisible(true)
		b.SetIcon(LoadIcon(icon))
		b.Action().BindFunc0(run)
		SetToolTip(b, i18n.T(text))
		return b
	}
	l.back = button("Scene nodes", "navigate-back", func() { e.SetDeviceLibraryVisible(false) })
	l.importButton = button("Import model...", "folder", func() {
		path := sceneDeviceFileDialog()
		if path != "" && !e.closed {
			if err := e.ImportDeviceTemplate(context.Background(), path); err != nil {
				e.report(err)
			}
		}
	})
	SetToolTip(l.importButton, "OBJ / glTF / GLB / .silkscene")
	l.search = NewSearchBox()
	l.search.SetParent(l)
	l.search.SetPlaceholder(i18n.T("Search devices"))
	l.search.SigTextChanged(func(string) { l.filter() })
	l.list = NewListWidget()
	l.list.SetParent(l)
	l.list.SetFillAvailable(true)
	l.list.SetIconVisible(true)
	l.list.SigSelectionChanged(func(interface{}, []int) { l.describe() })
	l.list.SetDragStartCallback(func(rows []int) ([]interface{}, DndAction) {
		if len(rows) != 1 {
			return nil, DndIgnore
		}
		d, ok := l.deviceAt(rows[0])
		if !ok || e.closed {
			return nil, DndIgnore
		}
		return []interface{}{sceneDeviceDrag{editor: e, id: d.ID, template: d.Template, generation: l.generation, revision: e.revision, prefix: l.prefix.Text()}}, DndCopy
	})
	l.prefixLabel = NewLabel(i18n.T("Tag prefix (empty = device ID)"))
	l.prefixLabel.SetParent(l)
	l.prefix = NewEdit()
	l.prefix.SetParent(l)
	l.details = NewLabel("")
	l.details.SetParent(l)
	l.insert = button("Insert device", "plus", func() {
		if d, ok := l.deviceAt(l.list.ActiveIndex()); ok {
			_, err := e.insertDevice(d, scene3d.Vec3{X: e.preview.target.X, Z: e.preview.target.Z}, l.prefix.Text())
			e.report(err)
		}
	})
	SetToolTip(l.list, i18n.T("Drag onto the ground plane; Shift snaps to 1 unit. No live points are started."))
	l.filter()
}
func (l *sceneDeviceLibrary) deviceAt(row int) (scene3d.DeviceDefinition, bool) {
	if row < 0 || row >= len(l.rows) {
		return scene3d.DeviceDefinition{}, false
	}
	index := l.rows[row]
	if index < 0 || index >= len(l.devices) {
		return scene3d.DeviceDefinition{}, false
	}
	return l.devices[index], true
}
func (l *sceneDeviceLibrary) filter() {
	selected := l.selectedID
	l.rows = nil
	l.list.Clear()
	query := strings.ToLower(strings.TrimSpace(l.search.Text()))
	for i, d := range l.devices {
		if strings.Contains(strings.ToLower(d.Name+" "+i18n.T(d.Name)+" "+d.ID), query) {
			l.rows = append(l.rows, i)
			l.list.Append(ListItem{Text: i18n.T(d.Name), Icon: LoadIcon("project"), Data: d.ID})
		}
	}
	index := -1
	for row, i := range l.rows {
		if l.devices[i].ID == selected {
			index = row
		}
	}
	if index < 0 && len(l.rows) > 0 {
		index = 0
	}
	l.list.SetActiveIndex(index)
	l.describe()
}
func (l *sceneDeviceLibrary) describe() {
	d, ok := l.deviceAt(l.list.ActiveIndex())
	l.insert.SetEnabled(ok && !l.editor.closed)
	if !ok {
		l.selectedID = ""
		l.details.SetText(i18n.T("No matching devices"))
		return
	}
	l.selectedID = d.ID
	stats := d.Template.Stats()
	text := i18n.Tf("%d nodes · %d bindings", stats.Nodes, stats.Bindings)
	tags := d.Template.Tags()
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		kind := "number"
		if tag.Boolean {
			kind = "bool"
		}
		names = append(names, tag.Name+" ("+kind+")")
	}
	details := text + "\n" + strings.Join(names, ", ")
	if warnings := d.Template.Warnings(); len(warnings) > 0 {
		details += "\n" + strings.Join(warnings, "\n")
	}
	l.details.SetText(text)
	SetToolTip(l.details, details)
}
func (l *sceneDeviceLibrary) Layout() {
	if l.back == nil || l.insert == nil {
		return
	}
	w, h := l.Size()
	w = max(0, w)
	h = max(0, h)
	gap := min(6.0, h/12)
	row := min(max(30.0, Theme().ItemHeight+6), max(0, h-6*gap)/6)
	half := max(0, (w-min(gap, w))/2)
	l.back.SetBounds(0, 0, half, row)
	l.importButton.SetBounds(min(w, half+gap), 0, half, row)
	y := row + gap
	l.search.SetBounds(0, y, w, row)
	y += row + gap
	listHeight := max(0, h-y-4*(row+gap))
	l.list.SetBounds(0, y, w, listHeight)
	y += listHeight + gap
	for _, widget := range []IWidget{l.prefixLabel, l.prefix, l.details, l.insert} {
		height := min(row, max(0, h-y))
		widget.SetBounds(0, min(h, y), w, height)
		y += height + gap
	}
}
func (l *sceneDeviceLibrary) cancelImport() {
	l.generation++
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	l.busy = false
	l.lastError = nil
	if l.importButton != nil {
		l.importButton.SetEnabled(!l.editor.closed)
		l.importButton.SetText(i18n.T("Import model..."))
	}
}

// ImportDeviceTemplate loads one model on the bounded worker executor. Completion
// only changes the shelf, never the authored document or live bindings. Replacing
// the shelf, starting another import, hiding it or closing retires old results.
func (e *SceneDocumentEditor) ImportDeviceTemplate(ctx context.Context, path string) error {
	if e.closed {
		return async.ErrClosed
	}
	if ctx == nil {
		return scene3d.ErrGeometry
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if path == "" {
		return scene3d.ErrUnsupported
	}
	e.ensureDeviceLibrary()
	l := e.library
	l.cancelImport()
	generation := l.generation
	child, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	l.busy = true
	l.importButton.SetEnabled(false)
	l.importButton.SetText(i18n.T("Loading model..."))
	loader := sceneDeviceLoader
	_, err := RunAsync(child, nil, func(ctx context.Context) (*scene3d.Template, error) { return loader(ctx, path, scene3d.LoadLimits{}) }, func(t *scene3d.Template, err error) {
		defer cancel()
		e.finishDeviceImport(generation, path, t, err)
	})
	if err != nil {
		l.cancelImport()
		return err
	}
	return nil
}
func (e *SceneDocumentEditor) nextDeviceID() string {
	used := make(map[string]bool, len(e.document.Nodes))
	for _, n := range e.document.Nodes {
		root, _, _ := strings.Cut(n.ID, "/")
		used[root] = true
	}
	for i := 1; ; i++ {
		id := fmt.Sprintf("device-%d", i)
		if !used[id] {
			return id
		}
	}
}
func (e *SceneDocumentEditor) insertDevice(d scene3d.DeviceDefinition, point scene3d.Vec3, prefix string) (*scene3d.TemplateInstance, error) {
	if e.closed {
		return nil, async.ErrClosed
	}
	id := e.nextDeviceID()
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		prefix = id + "."
	}
	return e.InstantiateDevice(d.Template, scene3d.InstanceOptions{ID: id, Name: d.Name, TagPrefix: prefix, Transform: scene3d.Translation(point)})
}

// InstantiateDevice merges the template and mapped inert bindings in one edit.
// It is also the public SDK entry point used by external static widget plugins.
func (e *SceneDocumentEditor) InstantiateDevice(template *scene3d.Template, options scene3d.InstanceOptions) (*scene3d.TemplateInstance, error) {
	if e.closed {
		return nil, async.ErrClosed
	}
	before, _ := e.scene.Snapshot()
	placed, err := e.session.InstantiateTemplate(context.Background(), template, options)
	if err != nil {
		return nil, err
	}
	e.syncSession(placed.RootID())
	if before.InstanceCount() == 0 {
		_ = e.preview.FitScene()
	}
	return placed, nil
}

func sceneDeviceSnap(p scene3d.Vec3, step float64) scene3d.Vec3 {
	if step > 0 {
		p.X = math.Round(p.X/step) * step
		p.Z = math.Round(p.Z/step) * step
	}
	return p
}

// IsDeviceImporting and DeviceImportError are UI-thread observations.
func (e *SceneDocumentEditor) IsDeviceImporting() bool {
	return !e.closed && e.library != nil && e.library.busy
}
func (e *SceneDocumentEditor) DeviceImportError() error {
	if e.library == nil {
		return nil
	}
	return e.library.lastError
}
func (e *SceneDocumentEditor) CancelDeviceImport() {
	if e.library != nil {
		e.library.cancelImport()
	}
}
func (e *SceneDocumentEditor) finishDeviceImport(generation uint64, path string, t *scene3d.Template, err error) {
	l := e.library
	if e.closed || generation != l.generation {
		return
	}
	l.cancel = nil
	l.busy = false
	l.importButton.SetEnabled(true)
	l.importButton.SetText(i18n.T("Import model..."))
	if err != nil {
		l.lastError = err
		e.report(err)
		return
	}
	id := fmt.Sprintf("import-%d", generation)
	used := make(map[string]bool, len(l.devices))
	for _, d := range l.devices {
		used[d.ID] = true
	}
	for used[id] {
		id += "-copy"
	}
	devices := append(e.DeviceLibrary(), scene3d.DeviceDefinition{ID: id, Name: filepath.Base(path), Template: t})
	if err = e.SetDeviceLibrary(devices); err != nil {
		l.lastError = err
		e.report(err)
		return
	}
	l.search.SetText("")
	for row, i := range l.rows {
		if l.devices[i].ID == id {
			l.list.SetActiveIndex(row)
			break
		}
	}
	l.describe()
}
