package gui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/paint"
)

var showProcessLibraryDialog = (*Dialog).ShowModal
var processLibraryOpenDialog = func() string { return OpenFileDialogForExtension("silkhmilibrary") }
var processLibrarySaveDialog = func() string { return SaveFileDialogForExtension("silkhmilibrary") }
var processLibraryLoad = hmi.LoadDeviceLibrary
var processLibrarySave = hmi.SaveDeviceLibrary
var processLibraryCaptureInput = ShowInputBox
var showProcessPointMapDialog = (*Dialog).ShowModal
var processLibraryConfirmReplace = func(parent IWidget) bool {
	return ShowMessageBox(parent, nil, i18n.T("Replace library?"), i18n.T("Replace the existing device library file?"), []string{"Replace", "Cancel"}) == "Replace"
}

func (e *ProcessEditor) DeviceLibrary() (hmi.DeviceLibrary, error) {
	if !e.deviceLibraryReady {
		e.deviceLibrary = hmi.BuiltinDeviceLibrary()
		e.deviceLibraryReady = true
	}
	return hmi.CloneDeviceLibrary(e.deviceLibrary)
}
func (e *ProcessEditor) SetDeviceLibrary(l hmi.DeviceLibrary) error {
	if e.closed {
		return ErrUIInvalid
	}
	copy, err := hmi.CloneDeviceLibrary(l)
	if err != nil {
		return err
	}
	e.deviceLibrary, e.deviceLibraryReady = copy, true
	e.deviceLibraryRevision++
	if e.deviceLibraryChanged != nil {
		e.deviceLibraryChanged()
	}
	return nil
}
func (e *ProcessEditor) DeviceLibraryJSON() string {
	l, err := e.DeviceLibrary()
	if err != nil {
		return ""
	}
	b, _ := l.JSON()
	return string(b)
}
func (e *ProcessEditor) SetDeviceLibraryJSON(s string) {
	l, err := hmi.ParseDeviceLibrary([]byte(s))
	if err == nil {
		err = e.SetDeviceLibrary(l)
	}
	e.setError(err)
}
func (v *ProcessDiagram) PlaceDevice(t hmi.DeviceTemplate, o hmi.DevicePlacement) (hmi.DeviceInstance, error) {
	if !v.canEdit() {
		return hmi.DeviceInstance{}, fmt.Errorf("hmi: authoring is locked")
	}
	v.OnPointerCancel()
	p, instance, err := hmi.PlaceDeviceTemplate(v.project, t, o)
	if err != nil {
		return hmi.DeviceInstance{}, v.applyCommand(p, err)
	}
	if err = v.applyCommand(p, nil); err != nil {
		return hmi.DeviceInstance{}, err
	}
	v.SelectNodes(instance.Nodes)
	return instance, nil
}

type processDeviceLibraryPane struct {
	Widget
	editor                               *ProcessEditor
	dialog                               *Dialog
	preview                              *ProcessDiagram
	search                               *SearchBox
	list                                 *ListWidget
	details, status                      *Label
	labels                               []*Label
	fields                               []*Edit
	capture, open, save, mapping, insert *Button
	rows                                 []int
	selected                             string
	pointMap                             map[string]string
	before                               string
	generation                           uint64
	busy, closed                         bool
	rebuilding                           bool
	cancel                               context.CancelFunc
}

func (e *ProcessEditor) ShowDeviceLibrary() {
	if e.closed || e.running || e.locked || e.closeDeviceLibrary != nil {
		return
	}
	e.Diagram.OnPointerCancel()
	p, err := newProcessDeviceLibraryPane(e)
	if err != nil {
		e.setError(err)
		return
	}
	defer p.Close()
	dlg := NewDialog(i18n.T("2D Device Library"), e)
	p.dialog = dlg
	dlg.SetContent(p)
	dlg.AddButton(i18n.T("Close"), DialogCancel)
	dlg.SetSize(1100, 780)
	dlg.SetInitialFocus(p.search)
	e.closeDeviceLibrary = func() { p.Close(); dlg.resolveCancel() }
	e.deviceLibraryChanged = p.filter
	defer func() { e.closeDeviceLibrary = nil; e.deviceLibraryChanged = nil }()
	_ = showProcessLibraryDialog(dlg)
	if err := dlg.LastError(); err != nil {
		e.setError(err)
	}
}

func newProcessDeviceLibraryPane(e *ProcessEditor) (*processDeviceLibraryPane, error) {
	if _, err := e.DeviceLibrary(); err != nil {
		return nil, err
	}
	p := &processDeviceLibraryPane{editor: e, before: e.ProjectJSON()}
	p.Widget.Init(p)
	p.search = NewSearchBox()
	p.search.SetParent(p)
	p.search.SetPlaceholder(i18n.T("Search devices"))
	p.search.SigTextChanged(func(string) { p.filter() })
	p.list = NewListWidget()
	p.list.SetParent(p)
	p.list.SetFillAvailable(true)
	p.list.SetIconVisible(true)
	p.list.SigSelectionChanged(func(interface{}, []int) { p.describe() })
	p.preview = NewProcessDiagram()
	p.preview.SetParent(p)
	p.preview.SetReadOnly(true)
	p.details = NewLabel("")
	p.details.SetWrap(true)
	p.details.SetParent(p)
	p.status = NewLabel(i18n.T("Inert preview. Independent points by default; wheel zoom / middle drag pan."))
	p.status.SetWrap(true)
	p.status.SetParent(p)
	button := func(text, icon string, fn func()) *Button {
		b := NewButton1(i18n.T(text), LoadIcon(icon))
		b.SetTextVisible(true)
		b.SetParent(p)
		b.Action().BindFunc0(fn)
		return b
	}
	p.capture = button("Capture selection", "plus", p.captureSelection)
	p.open = button("Open library", "folder", func() {
		if path := processLibraryOpenDialog(); path != "" && !p.closed {
			p.load(path)
		}
	})
	p.save = button("Save library", "save", func() {
		if path := processLibrarySaveDialog(); path != "" && !p.closed {
			p.saveTo(path, false)
		}
	})
	p.mapping = button("Point mapping...", "settings", p.editPointMap)
	p.insert = button("Insert device", "plus", p.place)
	for i, title := range []string{"Instance ID", "Point prefix (empty = ID.)", "Position X", "Position Y"} {
		label := NewLabel(i18n.T(title))
		label.SetParent(p)
		field := NewEdit()
		field.SetParent(p)
		if i >= 2 {
			field.SetText("20")
		}
		p.labels, p.fields = append(p.labels, label), append(p.fields, field)
	}
	p.filter()
	return p, nil
}
func (p *processDeviceLibraryPane) Draw(g paint.Painter) {
	g.SetBrush1(Theme().FormColor)
	g.Rectangle(0, 0, p.Width(), p.Height())
	g.Fill()
}
func (p *processDeviceLibraryPane) SizeHints() SizeHints {
	return SizeHints{Width: 1050, Height: 680, Policy: GrowHorizontal | GrowVertical}
}
func (p *processDeviceLibraryPane) OnResize() {
	if p.insert == nil {
		return
	}
	w, h := p.Size()
	p.search.SetBounds(8, 8, max(0, w-16), 30)
	for i, b := range []*Button{p.capture, p.open, p.save} {
		bw := max(0, (w-16)/3)
		b.SetBounds(8+float64(i)*bw, 44, max(0, bw-6), 30)
	}
	cols := max(1, min(4, int(max(0, w-16)/180)))
	rows := (4 + cols - 1) / cols
	bottom := max(88, h-40-float64(rows)*52)
	lw := min(260, max(120, w*.28))
	viewH := max(0, bottom-96)
	p.list.SetBounds(8, 88, max(0, lw-8), max(0, viewH-64))
	p.details.SetBounds(8, max(88, bottom-68), max(0, lw-8), min(60, viewH))
	p.preview.SetBounds(lw+8, 88, max(0, w-lw-16), viewH)
	p.preview.FitToView()
	bw := max(0, (w-16)/float64(cols))
	for i, field := range p.fields {
		x, y := 8+float64(i%cols)*bw, bottom+float64(i/cols)*52
		p.labels[i].SetBounds(x, y, max(0, bw-8), 18)
		field.SetBounds(x, y+18, max(0, bw-8), 28)
	}
	aw := min(180, max(0, (w-24)/3))
	p.mapping.SetBounds(max(0, w-2*aw-16), max(0, h-34), aw, 30)
	p.insert.SetBounds(max(0, w-aw-8), max(0, h-34), aw, 30)
	p.status.SetBounds(8, max(0, h-38), max(0, w-2*aw-32), 36)
}
func (p *processDeviceLibraryPane) device() (hmi.DeviceTemplate, bool) {
	i := p.list.ActiveIndex()
	if i < 0 || i >= len(p.rows) || p.rows[i] >= len(p.editor.deviceLibrary.Devices) {
		return hmi.DeviceTemplate{}, false
	}
	return p.editor.deviceLibrary.Devices[p.rows[i]], true
}
func (p *processDeviceLibraryPane) filter() {
	if p.closed {
		return
	}
	old := p.selected
	p.rebuilding = true
	p.rows = nil
	p.list.Clear()
	query := strings.ToLower(strings.TrimSpace(p.search.Text()))
	index := -1
	for i, d := range p.editor.deviceLibrary.Devices {
		if strings.Contains(strings.ToLower(d.ID+" "+d.Name+" "+i18n.T(d.Name)), query) {
			if d.ID == old {
				index = len(p.rows)
			}
			p.rows = append(p.rows, i)
			p.list.Append(ListItem{Text: i18n.T(d.Name), Icon: LoadIcon("project"), Data: d.ID})
		}
	}
	if index < 0 && len(p.rows) > 0 {
		index = 0
	}
	p.list.SetActiveIndex(index)
	p.rebuilding = false
	p.describe()
}
func (p *processDeviceLibraryPane) describe() {
	if p.insert == nil || p.rebuilding {
		return
	}
	d, ok := p.device()
	p.capture.SetEnabled(!p.busy && p.eligible() && len(p.editor.Diagram.SelectedNodes()) > 0)
	p.insert.SetEnabled(ok && !p.busy)
	p.mapping.SetEnabled(ok && !p.busy && len(d.Project.Points) > 0)
	if !ok {
		p.selected = ""
		p.details.SetText(i18n.T("No matching devices"))
		_ = p.preview.SetProject(hmi.Project{Version: hmi.Version, Width: 100, Height: 100})
		return
	}
	if p.selected != d.ID {
		p.pointMap = nil
		for n := 1; ; n++ {
			id := fmt.Sprintf("device%d", n)
			collision := false
			for _, node := range p.editor.Diagram.project.Nodes {
				collision = collision || strings.HasPrefix(node.ID, id+".")
			}
			if !collision {
				p.fields[0].SetText(id)
				break
			}
		}
	}
	p.selected = d.ID
	p.details.SetText(fmt.Sprintf("%s\n%d nodes · %d pipes · %d points", d.ID, len(d.Project.Nodes), len(d.Project.Links), len(d.Project.Points)))
	preview, err := hmi.CloneProject(d.Project)
	if err == nil {
		// Preview authored geometry without running dynamic visibility/position.
		for i := range preview.Nodes {
			preview.Nodes[i].Bindings = nil
		}
		err = p.preview.SetProject(preview)
	}
	if err != nil {
		p.report(err)
	}
	p.preview.FitToView()
}
func (p *processDeviceLibraryPane) report(err error) {
	if err != nil {
		p.status.SetText(err.Error())
	} else {
		p.status.SetText(i18n.T("Ready; no drivers or data sources were started."))
	}
}
func (p *processDeviceLibraryPane) eligible() bool {
	return !p.closed && !p.editor.closed && !p.editor.running && !p.editor.locked && p.before == p.editor.ProjectJSON()
}
func (p *processDeviceLibraryPane) place() {
	if !p.eligible() || p.busy {
		p.report(fmt.Errorf("hmi: document changed or authoring locked; reopen the library"))
		return
	}
	d, ok := p.device()
	if !ok {
		return
	}
	x, err := strconv.ParseFloat(strings.TrimSpace(p.fields[2].Text()), 64)
	if err != nil {
		p.report(err)
		return
	}
	y, err := strconv.ParseFloat(strings.TrimSpace(p.fields[3].Text()), 64)
	if err == nil {
		_, err = p.editor.Diagram.PlaceDevice(d, hmi.DevicePlacement{ID: p.fields[0].Text(), X: x, Y: y, PointPrefix: p.fields[1].Text(), Points: p.pointMap})
	}
	p.report(err)
	if err == nil && p.dialog != nil {
		p.dialog.resolveCancel() // placement is one document transaction, not a second commit
	}
}
func (p *processDeviceLibraryPane) captureSelection() {
	if !p.eligible() || p.busy {
		return
	}
	id, ok := processLibraryCaptureInput(p, nil, i18n.T("Capture device"), i18n.T("Unique template ID"), "custom-device")
	if !ok || !p.eligible() {
		return
	}
	tpl, err := hmi.CaptureDeviceTemplate(p.editor.Diagram.Project(), p.editor.Diagram.SelectedNodes(), id, id)
	if err == nil {
		var lib hmi.DeviceLibrary
		lib, err = p.editor.DeviceLibrary()
		if err == nil {
			lib.Devices = append(lib.Devices, tpl)
			err = p.editor.SetDeviceLibrary(lib)
		}
	}
	p.report(err)
	if err == nil {
		p.selected = id
		p.search.SetText("")
		p.filter()
	}
}
func (p *processDeviceLibraryPane) setBusy(b bool) {
	p.busy = b
	for _, w := range []IWidget{p.search, p.list, p.capture, p.open, p.save} {
		w.SetEnabled(!b)
	}
	for _, field := range p.fields {
		field.SetEnabled(!b)
	}
	p.describe()
}
func (p *processDeviceLibraryPane) load(path string) {
	if p.closed || p.busy {
		return
	}
	p.generation++
	gen, rev := p.generation, p.editor.deviceLibraryRevision
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.setBusy(true)
	_, err := RunAsync(ctx, nil, func(ctx context.Context) (hmi.DeviceLibrary, error) { return processLibraryLoad(ctx, path) }, func(l hmi.DeviceLibrary, err error) {
		defer cancel()
		if p.closed || p.editor.closed || gen != p.generation {
			return
		}
		p.cancel = nil
		p.setBusy(false)
		if p.editor.deviceLibraryRevision != rev {
			p.report(fmt.Errorf("hmi: library changed; reopen the file"))
			p.filter()
			return
		}
		if err == nil {
			err = p.editor.SetDeviceLibrary(l)
		}
		p.report(err)
		if err == nil {
			p.filter()
		}
	})
	if err != nil {
		cancel()
		p.cancel = nil
		p.setBusy(false)
		p.report(err)
	}
}
func (p *processDeviceLibraryPane) saveTo(path string, overwrite bool) {
	if p.closed || p.busy {
		return
	}
	l, err := p.editor.DeviceLibrary()
	if err != nil {
		p.report(err)
		return
	}
	p.generation++
	gen, rev := p.generation, p.editor.deviceLibraryRevision
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.setBusy(true)
	_, err = RunAsync(ctx, nil, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, processLibrarySave(ctx, path, l, overwrite)
	}, func(_ struct{}, err error) {
		defer cancel()
		if p.closed || p.editor.closed || gen != p.generation {
			return
		}
		p.cancel = nil
		p.setBusy(false)
		if p.editor.deviceLibraryRevision != rev {
			p.report(fmt.Errorf("hmi: library changed; save again"))
			return
		}
		if errors.Is(err, hmi.ErrDeviceLibraryExists) && !overwrite {
			if processLibraryConfirmReplace(p) && !p.closed {
				if rev != p.editor.deviceLibraryRevision {
					p.report(fmt.Errorf("hmi: library changed; save again"))
					return
				}
				p.saveTo(path, true)
			}
			return
		}
		p.report(err)
	})
	if err != nil {
		cancel()
		p.cancel = nil
		p.setBusy(false)
		p.report(err)
	}
}
func (p *processDeviceLibraryPane) Close() {
	if p.closed {
		return
	}
	p.closed = true
	// Independent child dialogs do not share the pane's native surface. Retire
	// them before their owner, including mapping/capture/overwrite confirmation.
	var retire func(IWidget)
	retire = func(root IWidget) {
		for _, child := range root.Children() {
			retire(child)
			if dialog, ok := child.(interface{ resolveCancel() }); ok {
				dialog.resolveCancel()
			}
		}
	}
	retire(p)
	p.generation++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	p.preview.Close()
}

func (p *processDeviceLibraryPane) editPointMap() {
	d, ok := p.device()
	if !ok || p.busy || p.closed {
		return
	}
	model := NewSimpleTableModel([]string{"Template point", "Type", "Category", "Unit", "Destination (blank = prefix)"})
	rev := p.editor.deviceLibraryRevision
	for _, point := range d.Project.Points {
		model.AddRow(point.Name, string(point.Kind), point.Category, point.Unit, p.pointMap[point.Name])
	}
	for i, width := range []float64{150, 80, 80, 80, 270} {
		model.SetColumnWidth(i, width)
	}
	table := new(processConfigTable)
	table.Init(table)
	table.SetModel(model)
	table.SetCellsEditable(true)
	box := NewVBox()
	box.AddWidget(table)
	hint := NewLabel(i18n.T("Explicit existing points must match type/category/unit. Blank creates prefixed points."))
	hint.SetWrap(true)
	box.AddWidget(hint)
	dlg := NewDialog(i18n.T("Device point mapping"), p)
	dlg.SetContent(box)
	dlg.AddButton(i18n.T("Cancel"), DialogCancel)
	apply := dlg.AddButton(i18n.T("Apply"), DialogOK)
	apply.Action().BindFunc0(func() {
		if !p.eligible() || rev != p.editor.deviceLibraryRevision || p.selected != d.ID {
			hint.SetText("Library or document changed; reopen point mapping")
			return
		}
		table.commitEdit()
		mapping := map[string]string{}
		points := map[string]hmi.Point{}
		for _, point := range d.Project.Points {
			points[point.Name] = point
		}
		seen := map[string]bool{}
		for i := 0; i < model.RowCount(); i++ {
			row := model.RowData(i)
			if len(row) < 5 {
				return
			}
			point, exists := points[row[0]]
			if !exists || seen[row[0]] || row[1] != string(point.Kind) || row[2] != point.Category || row[3] != point.Unit {
				hint.SetText("Template point metadata cannot be changed here")
				return
			}
			seen[row[0]] = true
			if row[4] != "" {
				mapping[point.Name] = row[4]
			}
		}
		if len(seen) != len(points) {
			hint.SetText("Template points are missing")
			return
		}
		p.pointMap = mapping
		dlg.resolveCancel()
	})
	dlg.SetSize(760, 450)
	dlg.SetInitialFocus(table)
	_ = showProcessPointMapDialog(dlg)
}

func init() {
	i18n.AddMany("zh", map[string]string{
		"2D Device Library": "2D 设备库", "Capture selection": "捕获所选子图", "Open library": "打开设备库", "Save library": "保存设备库",
		"Point mapping...": "点位映射...", "Instance ID": "实例 ID", "Point prefix (empty = ID.)": "点位前缀（空为 ID.）",
		"Position X": "位置 X", "Position Y": "位置 Y", "Capture device": "捕获设备", "Unique template ID": "唯一模板 ID",
		"Device point mapping": "设备点位映射", "Replace library?": "替换设备库？", "Replace the existing device library file?": "替换已有的设备库文件？",
		"Inert preview. Independent points by default; wheel zoom / middle drag pan.":            "静态预览；默认创建独立点位。滚轮缩放／中键平移。",
		"Ready; no drivers or data sources were started.":                                        "就绪；未启动驱动或数据采集。",
		"Explicit existing points must match type/category/unit. Blank creates prefixed points.": "复用已有点位需匹配类型、类别、单位；空白项按前缀创建独立点位。",
		"Pump station": "泵站", "Heat recovery": "热回收单元", "Valve station": "阀组",
	})
}
