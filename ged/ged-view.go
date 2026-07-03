package ged

import (
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/geom"
	"github.com/uk0/silk/graph"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
	"math"
	"sort"
	"strings"
	"time"
)

// clipItem stores copied widget info for paste operations.
type clipItem struct {
	factoryName string
	x, y, w, h  float64
	name        string
}

// clipboard holds the items from the most recent copy operation.
var clipboard []clipItem

// RunCallback is called when F5 is pressed. Set by the host application.
var RunCallback func()

// PreviewCallback is called when Ctrl+R is pressed. Set by the host application.
var PreviewCallback func()

// SwitchToDesignCallback is called when Ctrl+1 is pressed. Set by the host application.
var SwitchToDesignCallback func()

// SwitchToEditCallback is called when Ctrl+2 is pressed. Set by the host application.
var SwitchToEditCallback func()

// QuickOpenCallback is called when Ctrl+P is pressed. Set by the host application.
var QuickOpenCallback func()

// ShowCodePanelCallback is called after a widget is double-clicked on the
// canvas. The host application should bring the code panel tab to the front
// so the user can see and edit the widget's event handler code.
var ShowCodePanelCallback func()

// alignGuide represents a single alignment guide line shown during drag.
type alignGuide struct {
	x1, y1, x2, y2 float64
	snap           bool // true if the dragged widget snapped to this guide
}

// Alignment callbacks — set by the host application (design.go) so the
// GedView can trigger alignment operations defined in package main.
var (
	AlignLeftCallback    func()
	AlignRightCallback   func()
	AlignTopCallback     func()
	AlignBottomCallback  func()
	AlignCenterHCallback func()
	AlignCenterVCallback func()
	DistributeHCallback  func()
	DistributeVCallback  func()
)

// SelectionCallback is called when the GedView's selection changes.
type SelectionCallback func(items []graph.IItem)

type GedView struct {
	graph.GraphView
	selCallbacks []SelectionCallback
	snapEnabled  bool
	gridSize     float64
	showGrid     bool // paint the faint background grid overlay

	// Alignment guide state for drag operations
	alignGuides []alignGuide
	isDragging  bool
	dragOriginX float64
	dragOriginY float64

	// Space+Drag pan state
	spacePanReady bool
	isPanning     bool
	panStartX     float64
	panStartY     float64
}

func NewGedView() *GedView {
	p := new(GedView)
	p.Init(p)
	return p
}

// AddSelectionCallback registers an additional callback invoked when the
// canvas selection changes. This allows external components (like the code
// panel) to react to selection changes without replacing existing callbacks.
func (this *GedView) AddSelectionCallback(cb SelectionCallback) {
	this.selCallbacks = append(this.selCallbacks, cb)
}

func (this *GedView) Init(self gui.IWidget) {
	this.GraphView.Init(self)
	this.snapEnabled = true
	this.gridSize = 5.0
	this.SetScene(NewGedScene())
	this.SetZoomFactor(1)
	this.SetPageMarginVisible(false)
	this.AddStandardTools()
	this.SetPropertyConfigName("ged")

	// Update the status bar when selection changes, then dispatch to
	// additional callbacks (e.g. the code panel).
	this.SigSelectionChanged(func(s interface{}, sel *graph.Selection) {
		frame := gui.FindOwnerFrame(this)
		if frame == nil {
			return
		}
		sb := frame.StatusBar()
		if sb == nil {
			return
		}
		count := sel.Count()
		if count == 0 {
			sb.ShowMessage("Ready")
		} else if count == 1 {
			item := sel.ItemList()[0]
			if fw, ok := item.(*FakeWidget); ok {
				name := fw.WidgetFactoryName()
				if idx := strings.LastIndex(name, "."); idx >= 0 {
					name = name[idx+1:]
				}
				w, h := fw.Width(), fw.Height()
				sb.ShowMessage(fmt.Sprintf("Selected: %s (%.0fx%.0f mm)", name, w, h))
			} else {
				sb.ShowMessage("Selected: 1 item")
			}
		} else {
			sb.ShowMessage(fmt.Sprintf("Selected: %d items", count))
		}

		// Dispatch to additional selection callbacks
		items := sel.ItemList()
		for _, cb := range this.selCallbacks {
			cb(items)
		}
	})
}

func (this *GedView) GedScene() *GedScene {
	p, _ := this.Scene().(*GedScene)
	return p
}

// snapToGrid rounds v to the nearest multiple of step. A non-positive step
// disables snapping and returns v unchanged, so callers never divide by zero.
// Pure so the rounding rule is unit-testable without a live view.
func snapToGrid(v, step float64) float64 {
	if step <= 0 {
		return v
	}
	return math.Round(v/step) * step
}

// snapToGrid rounds a scene point to the nearest grid intersection when snap
// is enabled, routing each axis through the pure snapToGrid helper above.
func (this *GedView) snapToGrid(x, y float64) (float64, float64) {
	if !this.snapEnabled {
		return x, y
	}
	return snapToGrid(x, this.gridSize), snapToGrid(y, this.gridSize)
}

// SetSnapEnabled toggles snap-to-grid behavior.
func (this *GedView) SetSnapEnabled(enabled bool) {
	this.snapEnabled = enabled
}

// SnapEnabled returns whether snap-to-grid is active.
func (this *GedView) SnapEnabled() bool {
	return this.snapEnabled
}

// SetSnapToGrid toggles snap-to-grid (alias of SetSnapEnabled, matching the
// Qt Designer "Grid → Snap to grid" wording the toolbar/menu uses).
func (this *GedView) SetSnapToGrid(enabled bool) {
	this.snapEnabled = enabled
}

// IsSnapToGrid reports whether dragged/dropped widgets snap to the grid.
func (this *GedView) IsSnapToGrid() bool {
	return this.snapEnabled
}

// SetShowGrid toggles the faint background grid overlay drawn on the page.
func (this *GedView) SetShowGrid(show bool) {
	this.showGrid = show
}

// IsShowGrid reports whether the background grid overlay is drawn.
func (this *GedView) IsShowGrid() bool {
	return this.showGrid
}

// SetGridSize sets the snap grid spacing in mm.
func (this *GedView) SetGridSize(size float64) {
	if size > 0 {
		this.gridSize = size
	}
}

// SetGridStep sets the grid spacing in mm (alias of SetGridSize; the grid
// overlay and the snap rounding share this single step).
func (this *GedView) SetGridStep(step float64) {
	if step > 0 {
		this.gridSize = step
	}
}

// GridSize returns the current snap grid spacing in mm.
func (this *GedView) GridSize() float64 {
	return this.gridSize
}

func (this *GedView) OnDragEnter(x, y float64, dnd gui.IDndContext) {
	core.Debug("(this *GedView) OnDragEnter(x, y float64, dnd gui.IDndContext)")
	//this.MapToScene(x, y)

	if dnd.HasFormat("text/plain") {
		dnd.SetAction(gui.DndCopy)
	}
}

func (this *GedView) OnDragLeave() {

}

func (this *GedView) OnDragMove(x, y float64, dnd gui.IDndContext) {
	if !dnd.HasFormat("text/plain") {
		dnd.SetAction(gui.DndIgnore)
		return
	}
	dnd.SetAction(gui.DndCopy)
	return
	//
}

func (this *GedView) OnDrop(x, y float64, dnd gui.IDndContext) {
	factoryName, ok := dnd.Data("text/plain").(string)
	if !ok {
		dnd.SetAction(gui.DndIgnore)
		return
	}
	item, err := NewFakeWidgetFromFactory(factoryName)
	if err != nil {
		gui.ShowMessageBox(this, gui.LoadIcon("error"), "failed", err.Error(), []string{"@ok"})
		return
	}
	sx, sy := this.MapToScene(x, y)
	x1, y1 := this.snapToGrid(sx, sy)

	// Use type-specific default sizes instead of a fixed 30x8 mm
	w, h := defaultSizeForWidget(factoryName)
	item.SetBounds(x1, y1, w, h)

	// Call Layout so the embedded widget gets the correct pixel size
	item.Layout()

	// Nest the new widget into the container under the cursor, so layouts can
	// be built by drag-drop; fall back to the scene root when the drop is not
	// over a container. AddCommand attaches the fresh (parentless) item to any
	// parent — Redo does SetParent(parent), Undo does SetParent(nil) — so the
	// drop is one undoable action whether it lands in a container or at root.
	parent := graph.IItem(this.Scene())
	if c := this.containerUnder(sx, sy); c != nil {
		parent = c
	}
	cmd := graph.NewAddCommand()
	cmd.AddItem(item, parent)
	this.Scene().PushCommand(cmd)

	this.Self().Update()
}

// containerUnder returns the nearest container FakeWidget at the scene point
// (x, y), or nil when nothing container-like is under it. It reuses the same
// FindItemAt hit-test the canvas uses for click-selection (OnRightUp,
// onDoubleClick), then walks up from the topmost hit item to its closest
// container ancestor, so a drop onto a leaf that already sits inside a
// container still nests into that container.
func (this *GedView) containerUnder(x, y float64) graph.IItem {
	return nearestContainerAncestor(this.Scene().FindItemAt(x, y, nil))
}

// nearestContainerAncestor walks up the parent chain from hit (inclusive) and
// returns the first container FakeWidget, or nil if it reaches the root
// without finding one. Split out from containerUnder so the walk-up is
// unit-testable without a live view.
func nearestContainerAncestor(hit graph.IItem) graph.IItem {
	for it := hit; it != nil; it = it.Parent() {
		if isContainerItem(it) {
			return it
		}
	}
	return nil
}

// isContainerItem reports whether item is a FakeWidget wrapping a container
// widget that accepts dropped children. It reuses the framework's two existing
// container signals — the visual layout containers (layoutTypeLabel: VBox/HBox/
// GridLayout/FormLayout/Card/Accordion/StackedWidget/TabWidget/Splitter/
// ScrollArea) and the AddWidget containers (isSimpleAddContainer, which
// contributes GroupBox) — plus the Form/Dialog window containers. Leaf widgets
// (Button/Label/Edit/...) return false, so a drop over them falls back to the
// scene root in OnDrop.
func isContainerItem(item graph.IItem) bool {
	fw, ok := item.(*FakeWidget)
	if !ok {
		return false
	}
	name := fw.WidgetFactoryName()
	switch name {
	case "gui.Form", "gui.Dialog":
		return true
	}
	return layoutTypeLabel(name) != "" || isSimpleAddContainer(name)
}

// defaultSizeForWidget returns sensible default sizes (in mm) for dropped widgets.
func defaultSizeForWidget(factoryName string) (w, h float64) {
	shortName := factoryName
	if idx := strings.LastIndex(factoryName, "."); idx >= 0 {
		shortName = factoryName[idx+1:]
	}
	switch strings.ToLower(shortName) {
	case "button":
		return 25, 7
	case "label":
		return 30, 5
	case "edit":
		return 40, 6
	case "checkbox", "radiobutton":
		return 30, 6
	case "combobox":
		return 35, 6
	case "spinbox":
		return 25, 6
	case "progressbar":
		return 40, 5
	case "slider":
		return 40, 5
	case "table", "treeview", "listwidget":
		return 50, 30
	case "vbox", "hbox":
		return 40, 25
	case "groupbox":
		return 45, 30
	case "tabwidget", "stackedwidget":
		return 50, 30
	case "splitter":
		return 50, 25
	case "toolbar":
		return 50, 7
	case "statusbar":
		return 50, 6
	case "dialog", "form":
		return 60, 40
	case "gridlayout", "formlayout":
		return 45, 25
	case "scrollarea":
		return 45, 30
	case "linechart", "barchart", "scatterplot":
		return 60, 35
	case "piechart":
		return 40, 35
	case "gauge":
		return 35, 30
	case "toggleswitch":
		return 30, 7
	case "searchbox":
		return 40, 7
	case "numberinput":
		return 30, 7
	case "datepicker":
		return 35, 7
	case "colorpicker":
		return 30, 7
	case "rating":
		return 40, 6
	case "dropdownbutton":
		return 30, 7
	case "switchgroup":
		return 50, 7
	case "imageview":
		return 40, 30
	case "tag":
		return 20, 6
	case "badge":
		return 15, 15
	case "avatar":
		return 12, 12
	case "breadcrumb":
		return 50, 6
	case "link":
		return 25, 5
	case "labelseparator":
		return 50, 5
	case "placeholder":
		return 50, 30
	case "timeline":
		return 40, 40
	case "card":
		return 50, 35
	case "accordion":
		return 50, 35
	case "notificationpanel":
		return 50, 40
	default:
		return 30, 8
	}
}

// widgetEvents maps factory names to the list of events that can be bound.
var widgetEvents = map[string][]string{
	"gui.Button":      {"OnClick"},
	"gui.Edit":        {"OnChanged", "OnSubmit"},
	"gui.CheckBox":    {"OnToggled"},
	"gui.Slider":      {"OnValueChanged"},
	"gui.SpinBox":     {"OnValueChanged"},
	"gui.ComboBox":    {"OnSelected"},
	"gui.RadioButton": {"OnChanged"},
	"gui.ProgressBar": {},
	"gui.Label":       {},
}

// OnRightUp shows a context menu when the user right-clicks on the canvas.
func (this *GedView) OnRightUp(x, y float64) {
	sx, sy := this.MapToScene(x, y)
	item := this.Scene().FindItemAt(sx, sy, nil)

	menu := gui.NewMenu(true)

	if item != nil && item != this.Scene() {
		// Select the item if not already selected
		if !this.Selection().Contains(item) {
			this.Selection().Clear()
			this.Selection().Add(item)
		}

		// "Properties" menu item
		btnProps := menu.AddButton1("属性...", nil)
		btnProps.Action().BindFunc0(func() {
			this.showPropertyDialog(item)
		})

		// "Bind Event" submenu with per-widget-type events and current bindings
		fake, isFake := item.(*FakeWidget)
		eventMenu, _ := menu.AddSubMenu("绑定事件", nil, nil)

		if isFake {
			factoryName := fake.WidgetFactoryName()
			events := widgetEvents[factoryName]
			handlers := fake.EventHandlers()

			if len(events) > 0 {
				for _, evt := range events {
					evtName := evt
					// Show current binding status
					label := evtName + ": (未绑定)"
					if handlers != nil {
						if h, ok := handlers[evtName]; ok && h != "" {
							label = evtName + ": " + h + " ✓"
						}
					}
					btn := eventMenu.AddButton1(label, nil)
					btn.Action().BindFunc0(func() {
						this.bindEvent(item, evtName)
					})
				}
			} else {
				noEvt := eventMenu.AddButton1("(无可用事件)", nil)
				_ = noEvt
			}

			// Add "Remove Binding" option if there are active bindings
			if len(handlers) > 0 {
				eventMenu.AddSeparator()
				for evtKey, handlerVal := range handlers {
					ek := evtKey
					hv := handlerVal
					removeBtn := eventMenu.AddButton1("移除: "+ek+" → "+hv, nil)
					removeBtn.Action().BindFunc0(func() {
						fake.RemoveEventHandler(ek)
					})
				}
			}
		} else {
			// Fallback: generic events for unknown widget types
			for _, evt := range []string{"OnClick", "OnChanged", "OnSubmit", "OnSelected"} {
				evtName := evt
				btn := eventMenu.AddButton1(evtName, nil)
				btn.Action().BindFunc0(func() {
					this.bindEvent(item, evtName)
				})
			}
		}

		// "View Code" menu item -- triggers selection callbacks so the code
		// panel picks up this widget if it hasn't already.
		btnCode := menu.AddButton1("查看代码", nil)
		btnCode.Action().BindFunc0(func() {
			this.Selection().Clear()
			this.Selection().Add(item)
		})

		menu.AddSeparator()

		// Z-order submenu (Qt Designer's Raise / Lower / Bring-to-Front /
		// Send-to-Back) so overlapping widgets can be restacked. Each entry
		// applies the graph reorder to the whole selection then redraws.
		zMenu, _ := menu.AddSubMenu("层叠顺序", nil, nil)
		zMenu.AddButton1("上移一层", nil).Action().BindFunc0(func() {
			this.reorderSelection(graph.IItem.Raise)
		})
		zMenu.AddButton1("下移一层", nil).Action().BindFunc0(func() {
			this.reorderSelection(graph.IItem.Lower)
		})
		zMenu.AddButton1("置于顶层", nil).Action().BindFunc0(func() {
			this.reorderSelection(graph.IItem.BringToFront)
		})
		zMenu.AddButton1("置于底层", nil).Action().BindFunc0(func() {
			this.reorderSelection(graph.IItem.SendToBack)
		})

		// Align submenu (Qt Designer's Form → Align). The six align entries act
		// on the whole multi-selection; the two distribute entries even out the
		// gaps between widgets. Each entry routes through alignSelection, which
		// no-ops below its threshold (2 items for align, 3 for distribute) — the
		// entries stay visible so the menu layout is stable regardless of count.
		aMenu, _ := menu.AddSubMenu("对齐", nil, nil)
		aMenu.AddButton1("左对齐", nil).Action().BindFunc0(func() {
			this.alignSelection(AlignLeft)
		})
		aMenu.AddButton1("右对齐", nil).Action().BindFunc0(func() {
			this.alignSelection(AlignRight)
		})
		aMenu.AddButton1("水平居中", nil).Action().BindFunc0(func() {
			this.alignSelection(AlignHCenter)
		})
		aMenu.AddButton1("顶端对齐", nil).Action().BindFunc0(func() {
			this.alignSelection(AlignTop)
		})
		aMenu.AddButton1("底端对齐", nil).Action().BindFunc0(func() {
			this.alignSelection(AlignBottom)
		})
		aMenu.AddButton1("垂直居中", nil).Action().BindFunc0(func() {
			this.alignSelection(AlignVCenter)
		})
		aMenu.AddSeparator()
		aMenu.AddButton1("水平分布", nil).Action().BindFunc0(func() {
			this.alignSelection(DistributeH)
		})
		aMenu.AddButton1("垂直分布", nil).Action().BindFunc0(func() {
			this.alignSelection(DistributeV)
		})

		// "Lay Out" — wrap the current multi-selection in a VBox/HBox
		// container so the generated code arranges them via the layout
		// (Qt Designer's "Lay Out Vertically/Horizontally"). No-ops below
		// 2 selected items.
		lMenu, _ := menu.AddSubMenu("布局", nil, nil)
		lMenu.AddButton1("垂直布局 (VBox)", nil).Action().BindFunc0(func() {
			this.layOutSelection(false)
		})
		lMenu.AddButton1("水平布局 (HBox)", nil).Action().BindFunc0(func() {
			this.layOutSelection(true)
		})
		lMenu.AddSeparator()
		// "Break Layout" — the inverse of Lay Out: dissolve the selected
		// container(s), lifting their children back up to the container's
		// parent (Qt Designer's "Break Layout"). No-op when no selected
		// item is a container with children.
		lMenu.AddButton1("解散布局 (Break)", nil).Action().BindFunc0(func() {
			this.breakLayoutSelection()
		})

		menu.AddSeparator()

		// "Select All" — canvas-wide convenience, mirrors the Cmd/Ctrl+A
		// shortcut. Placed after the per-item Z-order/Align groups (it acts on
		// the page, not on the clicked widget) and routed through the same
		// selectAll() the keyboard path uses.
		btnSelectAll := menu.AddButton1("全选", nil)
		btnSelectAll.Action().BindFunc0(func() {
			this.selectAll()
		})

		menu.AddSeparator()

		// "Set Name" menu item
		btnName := menu.AddButton1("设置名称...", nil)
		btnName.Action().BindFunc0(func() {
			this.renameItem(item)
		})

		// "Lock/Unlock" menu item
		if isFake {
			lockLabel := "锁定"
			if fake.IsLocked() {
				lockLabel = "解锁"
			}
			btnLock := menu.AddButton1(lockLabel, nil)
			btnLock.Action().BindFunc0(func() {
				fake.SetLocked(!fake.IsLocked())
				this.Self().Update()
			})
		}

		menu.AddSeparator()

		// "Delete" menu item
		btnDelete := menu.AddButton1("删除", nil)
		btnDelete.Action().BindFunc0(func() {
			this.DeleteSelectedItems()
		})

	} else {
		// Right-click on empty canvas
		btnFormProps := menu.AddButton1("表单属性...", nil)
		btnFormProps.Action().BindFunc0(func() {
			this.showPropertyDialog(this.Scene())
		})

		// "Select All" is reachable from empty canvas too, so a designer can
		// grab every widget without first clicking one. Same selectAll() glue.
		btnSelectAll := menu.AddButton1("全选", nil)
		btnSelectAll.Action().BindFunc0(func() {
			this.selectAll()
		})
	}

	// Show popup menu at click position
	gx, gy := this.MapToGlobal(x, y)
	menu.ShowAsPopup(gx, gy, true)
}

// showPropertyDialog displays a property dialog for the selected item.
func (this *GedView) showPropertyDialog(obj interface{}) {
	core.Debug("showPropertyDialog for:", obj)
	// The property sheet is typically a tool view bound elsewhere;
	// trigger a selection change so the property sheet picks it up.
	if item, ok := obj.(graph.IItem); ok {
		if !this.Selection().Contains(item) {
			this.Selection().Clear()
			this.Selection().Add(item)
		}
	}
}

// bindEvent shows an input dialog asking for a handler function name, then
// stores the binding on the FakeWidget.
func (this *GedView) bindEvent(item graph.IItem, eventName string) {
	name, ok := gui.ShowInputBox(this, nil, "绑定事件",
		"处理函数名称 ("+eventName+"):", "on"+eventName)
	if ok && name != "" {
		if fake, ok := item.(*FakeWidget); ok {
			fake.SetEventHandler(eventName, name)
		}
	}
}

// DeleteSelectedItems removes all currently selected items from the scene as a
// single undoable DeleteCommand, so Ctrl+Z restores every widget at its
// original parent and z-order slot. The old bare Detach() loop dropped the
// items with no command on the stack, making deletion an unrecoverable
// data-loss operation.
func (this *GedView) DeleteSelectedItems() {
	sel := this.Selection()
	items := sel.ItemList()
	if len(items) == 0 {
		return
	}
	cmd := graph.NewDeleteCommand("Delete")
	for _, item := range items {
		cmd.Add(item)
	}
	this.Scene().PushCommand(cmd) // Push() calls Redo() → detaches the items.
	sel.Clear()
	this.Self().Update()
}

// zorderRec captures one Z-order step. On Redo we apply op(item) and
// snapshot the item's old sibling index; on Undo we walk the item back
// to that index via Raise/Lower (the only by-one primitives) so the
// round-trip is exact regardless of which op was used. Raise/Lower
// inverses ARE exact under op-pairing alone, but BringToFront/
// SendToBack are not (a middle-child raised to the front can't be
// restored by a single SendToBack), so we use the index snapshot as
// the single ground-truth inverse for every op.
type zorderRec struct {
	item    graph.IItem
	op      func(graph.IItem)
	inverse func(graph.IItem) // declared inverse op, kept for Text() reporting
	oldIdx  int               // sibling index before Redo, -1 until first apply
}

// zorderCommand is an undoable batch of Z-order steps. Records are
// applied in insertion order on Redo and reverse order on Undo, matching
// graph.ReparentCommand's contract; the isUndo guard panics on out-of-
// order calls the same way. Built in the pre-applied state — Scene().
// PushCommand calls Redo() to apply it.
type zorderCommand struct {
	records []zorderRec
	label   string
	isUndo  bool
}

func newZorderCommand(label string) *zorderCommand {
	return &zorderCommand{label: label}
}

func (cmd *zorderCommand) Add(item graph.IItem, op, inverse func(graph.IItem)) {
	cmd.records = append(cmd.records, zorderRec{item: item, op: op, inverse: inverse, oldIdx: -1})
}

func (cmd *zorderCommand) Count() int { return len(cmd.records) }

func (cmd *zorderCommand) Redo() {
	if cmd.isUndo {
		panic("illegal Redo()")
	}
	for i := 0; i < len(cmd.records); i++ {
		rec := &cmd.records[i]
		rec.oldIdx = rec.item.IndexInParent()
		rec.op(rec.item)
	}
	cmd.isUndo = true
}

func (cmd *zorderCommand) Undo() {
	if !cmd.isUndo {
		panic("illegal Undo()")
	}
	for i := len(cmd.records) - 1; i >= 0; i-- {
		rec := cmd.records[i]
		zorderMoveToIndex(rec.item, rec.oldIdx)
	}
	cmd.isUndo = false
}

func (cmd *zorderCommand) Text() string {
	if cmd.label != "" {
		return cmd.label
	}
	return fmt.Sprintf("Z-order %d item(s)", len(cmd.records))
}

// zorderMoveToIndex walks item back to the requested sibling index via
// Raise/Lower (the by-one primitives), so the resulting child order
// matches the pre-Redo state exactly. Targets outside the current
// sibling range are clamped; a no-parent item is a no-op.
func zorderMoveToIndex(item graph.IItem, target int) {
	parent := item.Parent()
	if parent == nil {
		return
	}
	if target < 0 {
		return
	}
	n := len(parent.Children())
	if target >= n {
		target = n - 1
	}
	for {
		cur := item.IndexInParent()
		if cur == target {
			return
		}
		if cur < target {
			item.Raise()
		} else {
			item.Lower()
		}
	}
}

// zorderInverse returns the inverse of a Z-order op together with a
// label for the UndoStack. Returns (nil, "") for an unrecognised op,
// which reorderSelection treats as a no-op so unknown ops never land on
// the stack. Go forbids `==` on func values, so we compare code-pointer
// strings via fmt.Sprintf("%p", ...) — every method expression has a
// distinct package-level symbol, so the printed pointer disambiguates.
func zorderInverse(op func(graph.IItem)) (func(graph.IItem), string) {
	key := fmt.Sprintf("%p", op)
	switch key {
	case fmt.Sprintf("%p", graph.IItem.Raise):
		return graph.IItem.Lower, "Raise"
	case fmt.Sprintf("%p", graph.IItem.Lower):
		return graph.IItem.Raise, "Lower"
	case fmt.Sprintf("%p", graph.IItem.BringToFront):
		return graph.IItem.SendToBack, "Bring to Front"
	case fmt.Sprintf("%p", graph.IItem.SendToBack):
		return graph.IItem.BringToFront, "Send to Back"
	}
	return nil, ""
}

// reorderSelection applies a Z-order operation (Raise/Lower/BringToFront/
// SendToBack) to every selected item via an undoable zorderCommand.
// Each record stores op plus its declared inverse for reporting; Undo
// walks the item back to its captured pre-Redo sibling index via the
// Raise/Lower primitives, so any op round-trips exactly (matching the
// ReparentCommand idiom from Lay Out / Break Layout in spirit, with a
// cheaper per-item snapshot — just one int — since Z-order never
// changes parent or coordinates).
func (this *GedView) reorderSelection(op func(graph.IItem)) {
	inverse, label := zorderInverse(op)
	if inverse == nil {
		return
	}
	items := this.Selection().ItemList()
	if len(items) == 0 {
		return
	}
	cmd := newZorderCommand(label)
	for _, item := range items {
		cmd.Add(item, op, inverse)
	}
	this.Scene().PushCommand(cmd)
	this.Self().Update()
}

// alignMode selects an align-or-distribute operation for a multi-selection.
// The first six entries reposition every rect onto a shared edge or centre
// line; the last two even out the gaps between rects along one axis.
type alignMode int

const (
	AlignLeft alignMode = iota
	AlignRight
	AlignHCenter
	AlignTop
	AlignBottom
	AlignVCenter
	DistributeH
	DistributeV
)

// alignRects returns a copy of rects repositioned per mode. Only X/Y are ever
// touched — widths and heights pass through unchanged, and the returned slice
// keeps the input order so callers can map results straight back onto the
// items they came from.
//
// Align{Left,Right,HCenter} snap each rect's left edge / right edge / centre to
// the selection's min-left / max-right / mean-centre on X; the Top/Bottom/
// VCenter trio is the Y analogue. Distribute{H,V} keep the two extreme rects
// fixed and re-space the interior ones (sorted along the axis) so consecutive
// gaps are equal; with fewer than three rects there is nothing to even out, so
// distribute is a no-op. Fewer than two rects is a no-op for every mode.
func alignRects(rects []geom.Rect, mode alignMode) []geom.Rect {
	out := make([]geom.Rect, len(rects))
	copy(out, rects)
	if len(rects) < 2 {
		return out
	}

	switch mode {
	case AlignLeft:
		left := rects[0].Left()
		for _, r := range rects[1:] {
			if r.Left() < left {
				left = r.Left()
			}
		}
		for i := range out {
			out[i].X = left
		}

	case AlignRight:
		right := rects[0].Right()
		for _, r := range rects[1:] {
			if r.Right() > right {
				right = r.Right()
			}
		}
		for i := range out {
			out[i].X = right - out[i].Width
		}

	case AlignHCenter:
		var sum float64
		for _, r := range rects {
			cx, _ := r.Center()
			sum += cx
		}
		cx := sum / float64(len(rects))
		for i := range out {
			out[i].X = cx - out[i].Width/2
		}

	case AlignTop:
		top := rects[0].Top()
		for _, r := range rects[1:] {
			if r.Top() < top {
				top = r.Top()
			}
		}
		for i := range out {
			out[i].Y = top
		}

	case AlignBottom:
		bottom := rects[0].Bottom()
		for _, r := range rects[1:] {
			if r.Bottom() > bottom {
				bottom = r.Bottom()
			}
		}
		for i := range out {
			out[i].Y = bottom - out[i].Height
		}

	case AlignVCenter:
		var sum float64
		for _, r := range rects {
			_, cy := r.Center()
			sum += cy
		}
		cy := sum / float64(len(rects))
		for i := range out {
			out[i].Y = cy - out[i].Height/2
		}

	case DistributeH:
		distributeAxis(out, true)

	case DistributeV:
		distributeAxis(out, false)
	}

	return out
}

// distributeAxis evens out the gaps between rects along one axis (horizontal
// when horizontal is true, vertical otherwise). The leftmost/topmost and
// rightmost/bottommost rects stay put; the interior rects are slid so every
// consecutive gap is identical. Operates in place on out. No-op for <3 rects.
func distributeAxis(out []geom.Rect, horizontal bool) {
	n := len(out)
	if n < 3 {
		return
	}

	// Sort indices by leading edge along the chosen axis (insertion sort —
	// selections are tiny). idx[k] is the rect that comes k-th along the axis.
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	lead := func(i int) float64 {
		if horizontal {
			return out[i].X
		}
		return out[i].Y
	}
	extent := func(i int) float64 {
		if horizontal {
			return out[i].Width
		}
		return out[i].Height
	}
	for i := 1; i < n; i++ {
		k := idx[i]
		j := i - 1
		for j >= 0 && lead(idx[j]) > lead(k) {
			idx[j+1] = idx[j]
			j--
		}
		idx[j+1] = k
	}

	// Total free space = span between the outer edges minus the widths of all
	// rects; share it equally across the n-1 gaps.
	first, last := idx[0], idx[n-1]
	span := lead(last) + extent(last) - lead(first)
	var occupied float64
	for _, i := range idx {
		occupied += extent(i)
	}
	gap := (span - occupied) / float64(n-1)

	cursor := lead(first) + extent(first)
	for k := 1; k < n-1; k++ {
		i := idx[k]
		pos := cursor + gap
		if horizontal {
			out[i].X = pos
		} else {
			out[i].Y = pos
		}
		cursor = pos + extent(i)
	}
}

// alignSelection reads the selected items' bounds, runs them through the
// pure alignRects helper, and pushes the per-item moves onto the
// UndoStack as a single MoveCommand. MoveCommand.AddItem takes absolute
// (toX, toY) so per-item deltas need no composite — one command carries
// every move and Ctrl+Z snaps everyone back to their original positions
// (the nudgeSelection idiom, but with N distinct targets instead of one
// shared delta).
//
// Below the mode's threshold (2 items for align, 3 for distribute)
// alignRects returns the bounds unchanged. We still build the command
// only when at least one item is actually moving, so a no-op mode never
// lands an empty MoveCommand on the stack.
func (this *GedView) alignSelection(mode alignMode) {
	items := this.Selection().ItemList()
	if len(items) < 2 {
		return
	}
	rects := make([]geom.Rect, len(items))
	for i, it := range items {
		rects[i] = it.Bounds1()
	}
	aligned := alignRects(rects, mode)
	cmd := graph.NewMoveCommand()
	for i, it := range items {
		oldX, oldY := it.Pos()
		if oldX == aligned[i].X && oldY == aligned[i].Y {
			continue
		}
		cmd.AddItem(it, aligned[i].X, aligned[i].Y)
	}
	if cmd.Count() == 0 {
		return
	}
	this.Scene().PushCommand(cmd)
	this.Self().Update()
}

// hasSelectedAncestor reports whether any ancestor of it is present in
// set — used to drop a selected item that sits inside another selected
// item before a layout reparent, preventing parent+child double-moves.
func hasSelectedAncestor(it graph.IItem, set map[graph.IItem]bool) bool {
	for p := it.Parent(); p != nil; p = p.Parent() {
		if set[p] {
			return true
		}
	}
	return false
}

// boundingBoxOf returns the smallest rect enclosing all of rects. Empty
// input returns the zero rect. Pure helper so layOutSelection's geometry
// is unit-testable without a scene.
func boundingBoxOf(rects []geom.Rect) geom.Rect {
	if len(rects) == 0 {
		return geom.Rect{}
	}
	minX, minY := rects[0].X, rects[0].Y
	maxX, maxY := rects[0].X+rects[0].Width, rects[0].Y+rects[0].Height
	for _, r := range rects[1:] {
		if r.X < minX {
			minX = r.X
		}
		if r.Y < minY {
			minY = r.Y
		}
		if r.X+r.Width > maxX {
			maxX = r.X + r.Width
		}
		if r.Y+r.Height > maxY {
			maxY = r.Y + r.Height
		}
	}
	return geom.Rect{X: minX, Y: minY, Width: maxX - minX, Height: maxY - minY}
}

// layOutSelection wraps the current multi-selection in a new VBox (or
// HBox when horizontal) container and reparents the selected widgets
// into it, so codegen arranges them through the container's AddWidget
// path. The container is sized to the selection's bounding box and the
// children are reparented in layout order (top-to-bottom for a VBox,
// left-to-right for an HBox). The new container becomes the selection.
//
// Like the other structural designer ops (Z-order, align, nudge), this
// mutates the scene directly without an undo command — wrapping it in a
// composite create+reparent command is a follow-up. No-op below 2
// selected items.
func (this *GedView) layOutSelection(horizontal bool) {
	raw := this.Selection().ItemList()

	// Guard the selection before reparenting anything:
	//   - the scene/page root must never be reparented (would orphan the
	//     whole design),
	//   - position-locked items are immutable (consistent with align/nudge),
	//   - an item whose ancestor is ALSO selected is dropped, so selecting a
	//     container together with one of its own children lays out only the
	//     container (no double-move / surprising re-nesting).
	// selSet holds only the selected *widget* items (not the scene root):
	// a child is suppressed when a selected CONTAINER ancestor is also in
	// the layout, but the scene being in the selection must not suppress
	// everything under it.
	selSet := make(map[graph.IItem]bool, len(raw))
	for _, it := range raw {
		if _, ok := it.(*FakeWidget); ok {
			selSet[it] = true
		}
	}
	items := make([]graph.IItem, 0, len(raw))
	for _, it := range raw {
		// Only real designer widgets are layout-eligible. This excludes
		// the scene/page root (a *GedScene, never a *FakeWidget) without
		// relying on identity comparison.
		if _, ok := it.(*FakeWidget); !ok {
			continue
		}
		if it.IsLockPos() { // position-locked widgets are immutable
			continue
		}
		if hasSelectedAncestor(it, selSet) {
			continue
		}
		items = append(items, it)
	}
	if len(items) < 2 {
		return
	}

	rects := make([]geom.Rect, len(items))
	for i, it := range items {
		rects[i] = it.Bounds1()
	}
	box := boundingBoxOf(rects)

	factory := "gui.VBox"
	if horizontal {
		factory = "gui.HBox"
	}
	container, err := NewFakeWidgetFromFactory(factory)
	if err != nil {
		return
	}
	container.SetBounds1(box)

	// Reparent in layout order: VBox stacks by Y, HBox by X.
	ordered := make([]graph.IItem, len(items))
	copy(ordered, items)
	sort.SliceStable(ordered, func(a, b int) bool {
		ra, rb := ordered[a].Bounds1(), ordered[b].Bounds1()
		if horizontal {
			return ra.X < rb.X
		}
		return ra.Y < rb.Y
	})

	// Build one undoable command: attach the container to the scene
	// first, then move each selected item into it. Parents are captured
	// now (pre-apply); PushCommand's Redo() applies the whole move, and
	// Ctrl+Z unwinds it (items return to their old parents, container
	// detaches).
	cmd := graph.NewReparentCommand("Lay Out")
	cmd.Add(container, nil, this.Scene())
	for _, it := range ordered {
		cmd.Add(it, it.Parent(), container)
	}
	this.Scene().PushCommand(cmd)
	container.Layout()

	sel := this.Selection()
	sel.Clear()
	sel.Add(container)
	this.Self().Update()
}

// breakLayoutSelection is the inverse of layOutSelection: for every
// selected container widget it lifts the children back up to the
// container's own parent and removes the now-empty container (Qt
// Designer's "Break Layout"). Children keep their absolute designer
// positions (containers use absolute coords in the canvas), so they
// stay put visually. The freed children become the new selection.
//
// Only *FakeWidget items that actually hold children are affected;
// anything else in the selection is ignored, so the action is a no-op
// when nothing container-like is selected. Like the other structural
// ops it mutates directly without an undo command.
func (this *GedView) breakLayoutSelection() {
	raw := this.Selection().ItemList()
	var freed []graph.IItem
	// One undoable command for the whole break: per container, lift each
	// child to the container's parent, then detach the container. Redo
	// applies it; Ctrl+Z re-nests the children and restores the container.
	cmd := graph.NewReparentCommand("Break Layout")
	for _, it := range raw {
		fake, ok := it.(*FakeWidget)
		if !ok || !fake.HasChildren() {
			continue
		}
		parent := fake.Parent()
		if parent == nil {
			continue
		}
		// Snapshot children before recording — building the command must
		// read the current (pre-apply) tree.
		kids := make([]graph.IItem, 0, len(fake.Children()))
		kids = append(kids, fake.Children()...)
		for _, c := range kids {
			cmd.Add(c, fake, parent) // child: container -> container's parent
			freed = append(freed, c)
		}
		cmd.Add(fake, parent, nil) // container: parent -> detached
	}
	if cmd.Count() == 0 {
		return
	}
	this.Scene().PushCommand(cmd)

	sel := this.Selection()
	sel.Clear()
	for _, c := range freed {
		sel.Add(c)
	}
	this.Self().Update()
}

// renameItem shows an input dialog to set the widget name of a FakeWidget.
func (this *GedView) renameItem(item graph.IItem) {
	if fake, ok := item.(*FakeWidget); ok {
		name, ok := gui.ShowInputBox(this, nil, "设置名称", "控件名称:", fake.WidgetName())
		if ok {
			fake.SetWidgetName(name)
			this.Self().Update()
		}
	}
}

// Double-click detection state.
var lastClickTime time.Time
var lastClickX, lastClickY float64

// OnLeftDown grabs focus, detects double-clicks for text editing, then
// delegates to GraphView for normal tool handling.
func (this *GedView) OnLeftDown(x, y float64) {
	// Space+Drag pan: intercept before any tool processing
	if this.spacePanReady {
		this.isPanning = true
		this.panStartX = x
		this.panStartY = y
		return
	}

	now := time.Now()
	if now.Sub(lastClickTime) < 400*time.Millisecond {
		dx := x - lastClickX
		dy := y - lastClickY
		if dx*dx+dy*dy < 25 { // within 5 px
			this.onDoubleClick(x, y)
			lastClickTime = time.Time{} // reset to avoid triple-click
			return
		}
	}
	lastClickTime = now
	lastClickX, lastClickY = x, y

	this.SetFocus()
	this.GraphView.OnLeftDown(x, y)

	// Record drag origin in scene coordinates for alignment guide computation
	sx, sy := this.MapToScene(x, y)
	this.dragOriginX = sx
	this.dragOriginY = sy
	this.isDragging = true
	this.alignGuides = nil
}

// onDoubleClick opens the code panel for the double-clicked widget and scrolls
// to its event handler. The selection callbacks notify the code panel, which
// loads the widget's code template and scrolls to the handler section.
func (this *GedView) onDoubleClick(x, y float64) {
	sx, sy := this.MapToScene(x, y)
	item := this.Scene().FindItemAt(sx, sy, nil)
	if item == nil {
		return
	}

	fake, ok := item.(*FakeWidget)
	if !ok {
		return
	}

	// Select the widget so the code panel picks it up via selection callbacks.
	this.Selection().Clear()
	this.Selection().Add(fake)

	// Notify any registered code-panel callbacks to focus and scroll.
	for _, cb := range this.selCallbacks {
		cb([]graph.IItem{fake})
	}

	// Bring the code panel to the front so the user can see the handler code.
	if ShowCodePanelCallback != nil {
		ShowCodePanelCallback()
	}
}

// getWidgetText returns the text of a widget if it has a Text() method.
func getWidgetText(w gui.IWidget) string {
	if w == nil {
		return ""
	}
	if t, ok := w.(interface{ Text() string }); ok {
		return t.Text()
	}
	return ""
}

// setWidgetText sets the text of a widget if it has a SetText(string) method.
func setWidgetText(w gui.IWidget, text string) {
	if w == nil {
		return
	}
	if t, ok := w.(interface{ SetText(string) }); ok {
		t.SetText(text)
	}
}

// OnKeyUp resets Space pan readiness when the Space key is released.
func (this *GedView) OnKeyUp(key int) {
	if key == gui.KeySpace {
		this.spacePanReady = false
		if this.isPanning {
			this.isPanning = false
		}
	}
}

// OnKeyDown handles keyboard shortcuts for the GED editor.
func (this *GedView) OnKeyDown(key int, repeat bool) {
	ctrl := gui.IsKeyDown(gui.KeyCtrl)
	alt := gui.IsKeyDown(gui.KeyMenu)

	// Space key enables pan mode
	if key == gui.KeySpace && !ctrl && !alt {
		this.spacePanReady = true
		return
	}

	switch {
	// Tab / Shift+Tab: cycle widget selection
	case key == gui.KeyTab:
		if gui.IsKeyDown(gui.KeyShift) {
			this.selectPrevWidget()
		} else {
			this.selectNextWidget()
		}

	case key == gui.KeyDelete || key == gui.KeyBackSpace:
		this.DeleteSelectedItems()

	// ESC clears the selection. Designer-tool muscle memory: ESC
	// is the universal "cancel context / deselect" key (JetBrains,
	// Figma, Sketch, even Photoshop). No-op when nothing is
	// selected — keeps the keystroke from triggering a layout
	// dirty for nothing.
	case key == gui.KeyEsc:
		if !this.Selection().IsEmpty() {
			this.Selection().Clear()
			this.Self().Update()
		}

	// Arrow keys nudge the selection by 1mm (nudgeGridStep mm with Shift).
	// Goes through the UndoStack so a stray arrow press in the middle of a
	// layout can be reversed with Cmd+Z. Designer-tool muscle memory: every
	// IDE from Qt Creator to Figma binds the arrow keys to a "fine move" of
	// the selection. The (key,shift)→(dx,dy) decision lives in the pure
	// nudgeDelta helper; the key is only consumed when something is selected,
	// so on empty canvas the arrows fall through to any default handling.
	case key == gui.KeyLeft, key == gui.KeyRight, key == gui.KeyUp, key == gui.KeyDown:
		if this.Selection().IsEmpty() {
			return
		}
		dx, dy, _ := nudgeDelta(key, gui.IsKeyDown(gui.KeyShift), 1, nudgeGridStep)
		this.nudgeSelection(dx, dy)

	case ctrl && (key == 'Z' || key == 'z'):
		if stack := this.Scene().UndoStack(); stack != nil {
			stack.Undo()
			this.Self().Update()
		}

	case ctrl && (key == 'Y' || key == 'y'):
		if stack := this.Scene().UndoStack(); stack != nil {
			stack.Redo()
			this.Self().Update()
		}

	// Cmd/Ctrl+A: select every selectable widget on the page. Ctrl is read
	// from IsKeyDown(KeyCtrl) up top (same modifier probe the nudge/undo cases
	// use), so this only fires with the modifier held and never collides with a
	// bare 'A' typed into an inline editor. Consumed here, ahead of the Alt
	// align block, so it can't fall through to anything else.
	case ctrl && (key == 'A' || key == 'a'):
		this.selectAll()

	case ctrl && (key == 'S' || key == 's'):
		this.GedScene().Save()

	case ctrl && (key == 'C' || key == 'c'):
		this.CopySelected()

	case ctrl && (key == 'V' || key == 'v'):
		this.PasteItems()

	case ctrl && (key == 'X' || key == 'x'):
		this.CopySelected()
		this.DeleteSelectedItems()

	// Cmd+D / Ctrl+D: duplicate selection in place. Re-uses the
	// CopySelected → PasteItems pipeline rather than introducing a
	// separate "duplicate" command — PasteItems already offsets the
	// new copies so they don't sit directly on top of the originals,
	// and the round-trip puts them on the UndoStack the same way an
	// explicit Cmd+C; Cmd+V would. Designer-tool muscle memory: Figma,
	// JetBrains, and Sketch all bind Cmd+D this way.
	case ctrl && (key == 'D' || key == 'd'):
		this.CopySelected()
		this.PasteItems()

	case ctrl && (key == 'P' || key == 'p'):
		if QuickOpenCallback != nil {
			QuickOpenCallback()
		}

	case ctrl && (key == 'R' || key == 'r'):
		if PreviewCallback != nil {
			PreviewCallback()
		}

	case key == gui.KeyF5:
		if RunCallback != nil {
			RunCallback()
		}

	case ctrl && key == '1':
		if SwitchToDesignCallback != nil {
			SwitchToDesignCallback()
		}

	case ctrl && key == '2':
		if SwitchToEditCallback != nil {
			SwitchToEditCallback()
		}

	// Alt+key alignment shortcuts
	case alt && (key == 'L' || key == 'l'):
		if AlignLeftCallback != nil {
			AlignLeftCallback()
		}
	case alt && (key == 'R' || key == 'r'):
		if AlignRightCallback != nil {
			AlignRightCallback()
		}
	case alt && (key == 'T' || key == 't'):
		if AlignTopCallback != nil {
			AlignTopCallback()
		}
	case alt && (key == 'B' || key == 'b'):
		if AlignBottomCallback != nil {
			AlignBottomCallback()
		}
	case alt && (key == 'C' || key == 'c'):
		if AlignCenterHCallback != nil {
			AlignCenterHCallback()
		}
	case alt && (key == 'M' || key == 'm'):
		if AlignCenterVCallback != nil {
			AlignCenterVCallback()
		}
	case alt && (key == 'H' || key == 'h'):
		if DistributeHCallback != nil {
			DistributeHCallback()
		}
	case alt && (key == 'V' || key == 'v'):
		if DistributeVCallback != nil {
			DistributeVCallback()
		}
	}
}

// sortedWidgetList returns the scene children sorted by Y then X (top-to-bottom, left-to-right).
func (this *GedView) sortedWidgetList() []graph.IItem {
	children := this.Scene().Children()
	if len(children) == 0 {
		return nil
	}
	// Copy and sort
	sorted := make([]graph.IItem, len(children))
	copy(sorted, children)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && (sorted[j].Y() > key.Y() || (sorted[j].Y() == key.Y() && sorted[j].X() > key.X())) {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	return sorted
}

// selectNextWidget selects the next widget on the canvas in tab order.
func (this *GedView) selectNextWidget() {
	sorted := this.sortedWidgetList()
	if len(sorted) == 0 {
		return
	}
	sel := this.Selection()
	items := sel.ItemList()
	idx := -1
	if len(items) == 1 {
		for i, it := range sorted {
			if it == items[0] {
				idx = i
				break
			}
		}
	}
	next := (idx + 1) % len(sorted)
	sel.Clear()
	sel.Add(sorted[next])
	this.Self().Update()
}

// selectPrevWidget selects the previous widget on the canvas in tab order.
func (this *GedView) selectPrevWidget() {
	sorted := this.sortedWidgetList()
	if len(sorted) == 0 {
		return
	}
	sel := this.Selection()
	items := sel.ItemList()
	idx := 0
	if len(items) == 1 {
		for i, it := range sorted {
			if it == items[0] {
				idx = i
				break
			}
		}
	}
	prev := (idx - 1 + len(sorted)) % len(sorted)
	sel.Clear()
	sel.Add(sorted[prev])
	this.Self().Update()
}

// nudgeGridStep is the larger Shift+arrow step (in mm). Qt Designer and
// every IDE bind plain arrows to a 1-unit "fine move" and Shift+arrow to a
// coarse grid-sized jump; 10 mm matches the coarse-grid muscle memory.
const nudgeGridStep = 10.0

// nudgeDelta maps an arrow key plus the Shift modifier to a movement delta.
// step is the fine (un-shifted) move; gridStep is the coarse Shift move.
// Left/Right move on X, Up/Down on Y (Up is negative — screen Y grows down).
// handled is false for any non-arrow key so the caller can fall through to
// other handling without consuming the keystroke. Pure so the (key,shift)
// → (dx,dy) decision is unit-testable without a live view.
func nudgeDelta(key int, shift bool, step, gridStep float64) (dx, dy float64, handled bool) {
	s := step
	if shift {
		s = gridStep
	}
	switch key {
	case gui.KeyLeft:
		return -s, 0, true
	case gui.KeyRight:
		return s, 0, true
	case gui.KeyUp:
		return 0, -s, true
	case gui.KeyDown:
		return 0, s, true
	}
	return 0, 0, false
}

// nudgeSelection shifts every selected item by (dx, dy) millimetres. The
// move is wrapped in a MoveCommand so it lands on the UndoStack alongside
// drag-moves — undo treats a Shift+Right press the same as a mouse drag.
//
// Note on undo: unlike alignSelection / reorderSelection (which mutate +
// Update with no command), nudge keeps the MoveCommand path. A clean move
// command already exists here and the arrow-key contract is test-locked to
// it (ged/nudge_test.go), so routing nudges through the UndoStack is the
// existing, deliberate choice — matching align's plain mutation would
// regress that behaviour.
//
// No-op if the selection is empty or contains only locked items;
// GenerateMoveCommand handles both edge cases internally and returns
// nil, which we forward as a quiet no-op rather than pushing an
// empty command.
func (this *GedView) nudgeSelection(dx, dy float64) {
	cmd := this.Selection().GenerateMoveCommand(dx, dy)
	if cmd == nil {
		return
	}
	this.Scene().UndoStack().Push(cmd)
	this.Self().Update()
}

// selectAll replaces the current selection with every selectable widget on the
// page. It iterates the scene's direct children (the dropped widgets) — the
// scene/page root is never in that list, so it is skipped for free — and adds
// only the ones that are visible and selectable, mirroring graph's
// TraversalCond_Selectable so a hidden or SetSelectable(false) item is left
// out exactly as a marquee drag would leave it out. Locked widgets stay in:
// IsLocked only pins position/size (IsLockPos/IsLockSize), it does not clear
// IsSelectable, and a designer needs to be able to select a locked widget to
// unlock it — so locked is NOT a skip reason here (unlike align/nudge, which
// skip IsLockPos because they move things).
//
// Undo: selection changes carry no command, matching every other select path
// in this file (single-click, Tab cycling, ESC clear) — none push onto the
// UndoStack. Cmd+A is just a viewport selection state change, so Cmd+Z should
// undo the last structural edit, not "the act of selecting".
func (this *GedView) selectAll() {
	sel := this.Selection()
	sel.Clear()
	// Recurse so widgets nested inside layout containers are selected too,
	// not just scene-level items.
	var addAll func(items []graph.IItem)
	addAll = func(items []graph.IItem) {
		for _, item := range items {
			if item.IsVisible() && item.IsSelectable() {
				sel.Add(item)
			}
			if item.HasChildren() {
				addAll(item.Children())
			}
		}
	}
	addAll(this.Scene().Children())
	this.Self().Update()
}

// CopySelected stores info about the selected FakeWidgets into the clipboard.
func (this *GedView) CopySelected() {
	clipboard = nil
	sel := this.Selection()
	for _, item := range sel.ItemList() {
		if fake, ok := item.(*FakeWidget); ok {
			clipboard = append(clipboard, clipItem{
				factoryName: fake.WidgetFactoryName(),
				x:           fake.X(),
				y:           fake.Y(),
				w:           fake.Width(),
				h:           fake.Height(),
				name:        fake.WidgetName(),
			})
		}
	}
}

// PasteItems creates new FakeWidgets from the clipboard, offset by
// one grid cell so the copies don't sit on top of the originals
// after snap-rounding. With the default 5mm grid, a fixed (2, 2)
// nudge plus snap actually rounded BACK to the original cell — the
// per-cell step here keeps the offset visible whatever the grid.
func (this *GedView) PasteItems() {
	if len(clipboard) == 0 {
		return
	}
	step := this.gridSize
	if step <= 0 {
		step = 5
	}
	// Track names already present in the scene plus names handed out
	// earlier in this same paste so a multi-item paste doesn't collide
	// with itself.
	taken := this.sceneWidgetNames()
	sel := this.Selection()
	sel.Clear()
	for _, ci := range clipboard {
		item, err := NewFakeWidgetFromFactory(ci.factoryName)
		if err != nil {
			continue
		}
		px, py := this.snapToGrid(ci.x+step, ci.y+step)
		item.SetBounds(px, py, ci.w, ci.h)
		name := uniqueWidgetName(ci.name, func(n string) bool { return taken[n] })
		taken[name] = true
		item.SetWidgetName(name)
		item.Layout()

		cmd := graph.NewAddCommand()
		cmd.AddItem(item, this.Scene())
		this.Scene().PushCommand(cmd)
		sel.Add(item)
	}
	this.Self().Update()
}

// sceneWidgetNames returns the set of non-empty widget names currently
// present in the scene. Used to keep pasted/duplicated widget names
// unique.
func (this *GedView) sceneWidgetNames() map[string]bool {
	taken := map[string]bool{}
	for _, item := range this.Scene().Children() {
		if fake, ok := item.(*FakeWidget); ok {
			if n := fake.WidgetName(); n != "" {
				taken[n] = true
			}
		}
	}
	return taken
}

// uniqueWidgetName returns base if it is free, otherwise the first
// "base_N" (N starting at 1) for which exists reports false. An empty
// base is returned unchanged: unnamed widgets carry no name to collide,
// and codegen synthesizes their field names separately. exists reports
// whether a candidate name is already in use.
func uniqueWidgetName(base string, exists func(string) bool) string {
	if base == "" || !exists(base) {
		return base
	}
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s_%d", base, i)
		if !exists(cand) {
			return cand
		}
	}
}

func (this *GedView) OpenFile(filename string) error {
	return this.GedScene().OpenFile(filename)
}

// ---------------------------------------------------------------------------
// Alignment guide system (Qt Creator-style snap lines during drag)
// ---------------------------------------------------------------------------

const alignTolerance = 1.5 // mm — snap distance for alignment guides

// computeAlignGuides calculates alignment guides between the dragged item(s)
// and all other items in the scene. dx, dy is the proposed movement delta.
func (this *GedView) computeAlignGuides(sel []graph.IItem, dx, dy float64) {
	this.alignGuides = nil
	if len(sel) == 0 {
		return
	}

	scene := this.GedScene()
	if scene == nil {
		return
	}

	sceneW, sceneH := scene.Size()
	allItems := scene.Children()

	// Build a set of selected items for fast lookup
	selSet := make(map[graph.IItem]bool, len(sel))
	for _, s := range sel {
		selSet[s] = true
	}

	// For each selected item, compute proposed edges and compare with others
	for _, dragging := range sel {
		px := dragging.X() + dx
		py := dragging.Y() + dy
		pw := dragging.Width()
		ph := dragging.Height()

		propLeft := px
		propRight := px + pw
		propCenterX := px + pw/2
		propTop := py
		propBottom := py + ph
		propCenterY := py + ph/2

		for _, other := range allItems {
			if selSet[other] {
				continue
			}

			ox, oy := other.X(), other.Y()
			ow, oh := other.Width(), other.Height()
			oLeft := ox
			oRight := ox + ow
			oCenterX := ox + ow/2
			oTop := oy
			oBottom := oy + oh
			oCenterY := oy + oh/2

			// Vertical guides (x-axis alignment)
			xEdges := [][2]float64{
				{propLeft, oLeft},
				{propLeft, oRight},
				{propRight, oLeft},
				{propRight, oRight},
				{propCenterX, oCenterX},
			}
			for _, pair := range xEdges {
				if math.Abs(pair[0]-pair[1]) < alignTolerance {
					this.alignGuides = append(this.alignGuides, alignGuide{
						x1: pair[1], y1: 0,
						x2: pair[1], y2: sceneH,
						snap: true,
					})
				}
			}

			// Horizontal guides (y-axis alignment)
			yEdges := [][2]float64{
				{propTop, oTop},
				{propTop, oBottom},
				{propBottom, oTop},
				{propBottom, oBottom},
				{propCenterY, oCenterY},
			}
			for _, pair := range yEdges {
				if math.Abs(pair[0]-pair[1]) < alignTolerance {
					this.alignGuides = append(this.alignGuides, alignGuide{
						x1: 0, y1: pair[1],
						x2: sceneW, y2: pair[1],
						snap: true,
					})
				}
			}
		}
	}
}

// drawGrid paints a faint dot at every grid intersection across the page area.
// Called in scene-mm coordinate space (same transform as drawAlignGuides), so
// the dot spacing tracks the pan/zoom the canvas already applied. Dots are
// drawn with a tiny zero-width-pen cross per intersection rather than filled
// circles to stay cheap and crisp at any zoom; only the visible page rect is
// walked so the loop count scales with page size / gridSize, not the canvas.
func (this *GedView) drawGrid(g paint.Painter) {
	step := this.gridSize
	if step <= 0 {
		return
	}
	w, h := this.SceneSizeMm()

	// Very light grey so the grid is a hint, not a feature competing with the
	// widgets. Zero pen width = 1px hairline regardless of zoom.
	g.SetPen1(paint.Color{0, 0, 0, 28}, 0)

	const tick = 0.3 // mm — half-length of each dot's cross arms
	for x := step; x < w; x += step {
		for y := step; y < h; y += step {
			g.MoveTo(x-tick, y)
			g.LineTo(x+tick, y)
			g.MoveTo(x, y-tick)
			g.LineTo(x, y+tick)
		}
	}
	g.Stroke()
}

// drawAlignGuides renders the active alignment guide lines.
// Called in scene-mm coordinate space.
func (this *GedView) drawAlignGuides(g paint.Painter) {
	if len(this.alignGuides) == 0 {
		return
	}

	// Semi-transparent blue, hairline width (0 = 1px regardless of zoom)
	g.SetPen1(paint.Color{66, 133, 244, 180}, 0)

	for _, guide := range this.alignGuides {
		g.MoveTo(guide.x1, guide.y1)
		g.LineTo(guide.x2, guide.y2)
		g.Stroke()
	}
}

// Draw overrides GraphView.Draw to add the background grid and the alignment
// guide overlay. The grid (when enabled) is painted first as a faint backdrop
// hint; the active drag guides are painted over it.
func (this *GedView) Draw(g paint.Painter) {
	this.GraphView.Draw(g)

	wantGrid := this.showGrid && this.gridSize > 0
	if !wantGrid && len(this.alignGuides) == 0 {
		return
	}

	// Clip to the page area so neither the grid nor the guides bleed into the
	// padding region, then re-enter scene-mm coordinate space (the same
	// transform GraphView.Draw uses for the scene) so both overlays honour the
	// current pan/zoom.
	g.Save()
	pw, ph := this.PageSizePx(true, this.ZoomFactor())
	pLeft, pTop := this.PageOriginPx()
	g.Rectangle(pLeft-this.ScrollX(), pTop-this.ScrollY(), pw, ph)
	g.Clip()

	x0, y0 := this.SceneOriginPx()
	g.Translate(x0-this.ScrollX(), y0-this.ScrollY())
	pageScale := gui.ScreenDpmm() * this.ZoomFactor()
	g.Scale(pageScale, pageScale)

	if wantGrid {
		this.drawGrid(g)
	}
	this.drawAlignGuides(g)
	g.Restore()
}

// OnMouseMove overrides GraphView.OnMouseMove to compute alignment guides
// while dragging selected items.
func (this *GedView) OnMouseMove(x, y float64) {
	// Space+Drag pan: adjust scroll offset
	if this.isPanning {
		dx := x - this.panStartX
		dy := y - this.panStartY
		this.SetScrollX(this.ScrollX() - dx)
		this.SetScrollY(this.ScrollY() - dy)
		this.panStartX = x
		this.panStartY = y
		this.Self().Update()
		return
	}

	this.GraphView.OnMouseMove(x, y)

	// Update cursor for resize handles when not dragging
	if !this.isDragging {
		sx, sy := this.MapToScene(x, y)
		decor, handle := this.FindHandleAt(sx, sy)
		if decor != nil && handle != 0 {
			gui.SetOverrideCursor(decor.HandleCursor(handle))
		} else {
			gui.SetOverrideCursor(nil)
		}
	}

	if !this.isDragging {
		return
	}

	sel := this.Selection()
	if sel == nil || sel.Count() == 0 {
		this.alignGuides = nil
		return
	}

	sx, sy := this.MapToScene(x, y)
	dx := sx - this.dragOriginX
	dy := sy - this.dragOriginY

	this.computeAlignGuides(sel.ItemList(), dx, dy)
}

// OnLeftUp overrides GraphView.OnLeftUp to clear alignment guides when
// the drag ends.
func (this *GedView) OnLeftUp(x, y float64) {
	// End Space+Drag pan
	if this.isPanning {
		this.isPanning = false
		return
	}

	wasDragging := this.isDragging
	this.GraphView.OnLeftUp(x, y)
	this.isDragging = false
	this.alignGuides = nil

	// Snap the dragged widget(s) onto grid intersections once the base
	// MovePart has committed the move command. We snap after the commit
	// (rather than rewriting the delta mid-drag, which lives in the graph
	// MovePart we don't own) so the final resting position lands on the
	// grid. The align guides drawn during the drag are purely visual and
	// don't move the widget, so grid snap and the guides compose cleanly.
	if wasDragging {
		this.snapSelectionToGrid()
	}

	this.Self().Update()
}

// snapSelectionToGrid rounds every position-unlocked selected item's top-left
// corner onto the nearest grid intersection. No-op when snap is disabled or
// the step is non-positive. Applied directly (mutate + Update) like
// alignSelection / reorderSelection — it tidies the post-drag resting place
// rather than introducing its own move command.
func (this *GedView) snapSelectionToGrid() {
	if !this.snapEnabled || this.gridSize <= 0 {
		return
	}
	for _, item := range this.Selection().ItemList() {
		if item.IsLockPos() {
			continue
		}
		x, y := item.Pos()
		nx := snapToGrid(x, this.gridSize)
		ny := snapToGrid(y, this.gridSize)
		if nx != x || ny != y {
			item.SetPos(nx, ny)
		}
	}
}
