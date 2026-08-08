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

// clipboard holds one serialized subtree per root of the most recent copy, in
// the very form SaveDesign writes to disk. Keeping the clipboard on the design
// format rather than a hand-picked field list is what makes a copy carry the
// widget's properties, event bindings and children: a second, smaller
// serializer beside it would drift from the first and silently drop whatever it
// had not learned about yet.
var clipboard []*core.TDoc

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

// DocClosedCallback is called with a design view the dock has just closed. Set
// by the host application, which is where anything keyed on an open document
// has to be retired; see GedView.Close.
var DocClosedCallback func(*GedView)

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
	showGrid     bool // paint the faint background dot overlay on top of the page

	// Alignment guide state for drag operations
	alignGuides []alignGuide
	isDragging  bool
	dragOriginX float64
	dragOriginY float64

	// Rulers and user-placed guides (see guides.go). The guides themselves
	// live on the scene; the view owns their visibility and the drag gesture.
	showGuides bool
	guideDrag  guideDragState

	// Re-parent state for a canvas drag. dropContainer is the container the
	// release would nest the selection into, nil meaning the form root — the
	// same fallback a palette drop uses. dragMovesItems records whether the
	// gesture is a widget drag at all, so neither a marquee nor a plain
	// selection click re-parents anything.
	dropContainer  graph.IItem
	dragMovesItems bool

	// gestureSeq groups the commands pushed by one continuous keyboard
	// gesture so they collapse into a single undo step; see beginGesture.
	gestureSeq uint64

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
	this.showGuides = true
	this.SetScene(NewGedScene())
	this.SetZoomFactor(1)
	this.SetPageMarginVisible(false)
	// Reserve the ruler strips (guides.go) in the view padding, on every side
	// so the centred page clears them too. Without it a scrolled or zoomed
	// page comes to rest under the rulers and the widgets in the form's
	// top-left corner cannot be clicked at all.
	this.SetPaddingPx(rulerThicknessPx+2, rulerThicknessPx+2, rulerThicknessPx+2, rulerThicknessPx+2)
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

// Undo steps the design's command stack back one command and reports the
// design changed. Running a command backwards pushes nothing, so the signal
// PushCommand raises never fires for an undo — every listener on it would
// otherwise keep showing the state Ctrl+Z just left.
func (this *GedView) Undo() {
	stack := this.Scene().UndoStack()
	if stack == nil {
		return
	}
	stack.Undo()
	if scene := this.GedScene(); scene != nil {
		scene.NotifyDesignChanged()
	}
	this.Self().Update()
}

// Redo is Undo in the other direction.
func (this *GedView) Redo() {
	stack := this.Scene().UndoStack()
	if stack == nil {
		return
	}
	stack.Redo()
	if scene := this.GedScene(); scene != nil {
		scene.NotifyDesignChanged()
	}
	this.Self().Update()
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
	g := this.gridModel()
	if !g.Snap {
		return x, y
	}
	return snapToGrid(x, g.Pitch), snapToGrid(y, g.Pitch)
}

// gridModel returns the grid of the scene under the view. GraphView.SetScene
// accepts any IScene, so the accessors below fall back to the defaults rather
// than assuming a GedScene is attached.
func (this *GedView) gridModel() GridModel {
	if s := this.GedScene(); s != nil {
		return s.Grid()
	}
	return defaultGridModel()
}

// setGridModel writes the grid back onto the scene. A view without a GedScene
// has nowhere to store it, so the change is dropped rather than cached here —
// caching would resurrect the view-local copy the model replaced.
func (this *GedView) setGridModel(g GridModel) {
	if s := this.GedScene(); s != nil {
		s.SetGrid(g)
	}
}

// SetSnapEnabled toggles snap-to-grid behavior.
func (this *GedView) SetSnapEnabled(enabled bool) {
	g := this.gridModel()
	g.Snap = enabled
	this.setGridModel(g)
}

// SnapEnabled returns whether snap-to-grid is active.
func (this *GedView) SnapEnabled() bool {
	return this.gridModel().Snap
}

// SetSnapToGrid toggles snap-to-grid (alias of SetSnapEnabled, matching the
// Qt Designer "Grid → Snap to grid" wording the toolbar/menu uses).
func (this *GedView) SetSnapToGrid(enabled bool) {
	this.SetSnapEnabled(enabled)
}

// IsSnapToGrid reports whether dragged/dropped widgets snap to the grid.
func (this *GedView) IsSnapToGrid() bool {
	return this.SnapEnabled()
}

// SetShowGrid toggles the faint background dot overlay drawn on the page. This
// is the overlay's own opt-in, on top of the scene's grid visibility.
func (this *GedView) SetShowGrid(show bool) {
	this.showGrid = show
}

// IsShowGrid reports whether the background dot overlay is opted in.
func (this *GedView) IsShowGrid() bool {
	return this.showGrid
}

// SetGridSize sets the grid spacing in mm.
func (this *GedView) SetGridSize(size float64) {
	if size <= 0 {
		return
	}
	g := this.gridModel()
	g.Pitch = size
	this.setGridModel(g)
}

// SetGridStep sets the grid spacing in mm (alias of SetGridSize; the canvas
// grid, the snap rounding and the coarse nudge share this single step).
func (this *GedView) SetGridStep(step float64) {
	this.SetGridSize(step)
}

// GridSize returns the current grid spacing in mm.
func (this *GedView) GridSize() float64 {
	return this.gridModel().Pitch
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

	// Claim the drop. Both backends offer it to the widget under the cursor and
	// then to its ancestors, stopping at the first one that sets an action, so a
	// canvas that stays on DndIgnore reads as a refusal: the drop it already
	// consumed would be handed on to the enclosing Dock and Frame, and the
	// palette would be told DndIgnore.
	dnd.SetAction(gui.DndCopy)

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

// nearestDropContainer picks the container a canvas drag may drop into: the
// deepest container on hit's parent chain that the moving set can legally nest
// into. `moving` reports whether an item belongs to that set; nil means no
// eligible container, i.e. the form root.
//
// Unlike nearestContainerAncestor the walk cannot stop at the first container:
// a container only qualifies if nothing ABOVE it is moving either, so a moving
// ancestor found later discards whatever was found below it. That is what keeps
// a selection from being dropped into itself or into something it carries along
// — either would build a parent cycle.
func nearestDropContainer(hit graph.IItem, moving func(graph.IItem) bool) graph.IItem {
	var found graph.IItem
	for it := hit; it != nil; it = it.Parent() {
		if moving(it) {
			found = nil
			continue
		}
		if found == nil && isContainerItem(it) {
			found = it
		}
	}
	return found
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

// eventsForItem returns the events the designer may bind on item, which is
// what codegen can emit for it (nothing for a non-widget item). It replaced a
// second event table maintained here: that table offered gui.Edit.OnSubmit and
// gui.ComboBox.OnSelected, events codegen has no binding for, so the menu
// ticked them as bound while generation dropped them.
func eventsForItem(item graph.IItem) []string {
	fake, ok := item.(*FakeWidget)
	if !ok {
		return nil
	}
	return AvailableEvents(fake.WidgetFactoryName())
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
			events := eventsForItem(item)
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
			// A non-widget item has no generated code, so no event of it can
			// be bound. The old fallback listed four generic names here that
			// codegen could never emit for anything.
			eventMenu.AddButton1("(无可用事件)", nil)
		}

		// "变形为" submenu — retype this widget in place. Single-selection
		// only: the candidates come from the clicked widget's own palette
		// category, so a mixed selection has no one answer.
		if isFake && this.Selection().Count() == 1 {
			if cands := morphCandidates(fake.WidgetFactoryName()); len(cands) > 0 {
				morphMenu, _ := menu.AddSubMenu("变形为", nil, nil)
				for _, c := range cands {
					target := c
					morphMenu.AddButton1(widgetFriendlyName(target), nil).Action().BindFunc0(func() {
						this.morphSelection(fake, target)
					})
				}
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
		// on the whole multi-selection; 居中 centres it in its container; the two
		// distribute entries even out the gaps between widgets. Each entry routes
		// through AlignSelection, which no-ops below its threshold (3 items for
		// distribute) — the entries stay visible so the menu layout is stable
		// regardless of count.
		aMenu, _ := menu.AddSubMenu("对齐", nil, nil)
		aMenu.AddButton1("左对齐", nil).Action().BindFunc0(func() {
			this.AlignSelection(AlignLeft)
		})
		aMenu.AddButton1("右对齐", nil).Action().BindFunc0(func() {
			this.AlignSelection(AlignRight)
		})
		aMenu.AddButton1("水平居中", nil).Action().BindFunc0(func() {
			this.AlignSelection(AlignHCenter)
		})
		aMenu.AddButton1("顶端对齐", nil).Action().BindFunc0(func() {
			this.AlignSelection(AlignTop)
		})
		aMenu.AddButton1("底端对齐", nil).Action().BindFunc0(func() {
			this.AlignSelection(AlignBottom)
		})
		aMenu.AddButton1("垂直居中", nil).Action().BindFunc0(func() {
			this.AlignSelection(AlignVCenter)
		})
		aMenu.AddButton1("居中", nil).Action().BindFunc0(func() {
			this.AlignSelection(AlignCenter)
		})
		aMenu.AddSeparator()
		aMenu.AddButton1("水平分布", nil).Action().BindFunc0(func() {
			this.AlignSelection(DistributeH)
		})
		aMenu.AddButton1("垂直分布", nil).Action().BindFunc0(func() {
			this.AlignSelection(DistributeV)
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

		// Lock toggles. Position and size are separate guards in graph — a
		// move skips IsLockPos items, a resize skips IsLockSize ones — so
		// pinning a widget's place while still sizing it is a state the old
		// all-or-nothing 锁定 entry could not express. Both act on the whole
		// selection and read their state at build time, which is safe because
		// the menu is rebuilt on every right-click.
		lockPos, lockSize := selectionLockState(this.Selection().ItemList())
		btnLockPos := menu.AddButton1(lockMenuLabel("锁定位置", lockPos), nil)
		btnLockPos.Action().BindFunc0(func() {
			this.setSelectionLockPos(!lockPos)
		})
		btnLockSize := menu.AddButton1(lockMenuLabel("锁定尺寸", lockSize), nil)
		btnLockSize.Action().BindFunc0(func() {
			this.setSelectionLockSize(!lockSize)
		})

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
// stores the binding on the FakeWidget. The name goes through the same
// validation as the property sheet's event rows, so neither entry point can
// record a handler that generated code cannot call.
func (this *GedView) bindEvent(item graph.IItem, eventName string) {
	name, ok := gui.ShowInputBox(this, nil, "绑定事件",
		"处理函数名称 ("+eventName+"):", "on"+eventName)
	if ok && name != "" {
		if fake, ok := item.(*FakeWidget); ok {
			fake.SetEventHandlerChecked(eventName, name)
		}
	}
}

// lockMenuLabel marks a toggle entry's current state. A popup-menu button
// never renders Action.SetChecked — the theme's popup branch reads only hover
// and sub-popup state — so the check has to live in the text, the way the
// bound-event entries mark a binding. The pad keeps both states aligned.
func lockMenuLabel(text string, on bool) string {
	if on {
		return "✓ " + text
	}
	return "  " + text
}

// selectionLockState reports whether EVERY item carries the position / size
// lock. A mixed selection reads as unlocked, so the next click locks all of it
// rather than releasing the half that was already locked.
func selectionLockState(items []graph.IItem) (pos, size bool) {
	if len(items) == 0 {
		return false, false
	}
	pos, size = true, true
	for _, item := range items {
		if !item.IsLockPos() {
			pos = false
		}
		if !item.IsLockSize() {
			size = false
		}
	}
	return
}

// setSelectionLockPos pins (or releases) the position of every selected item.
// Deliberately not undoable: a lock guards future edits instead of changing the
// design, so Ctrl+Z stays aimed at the last real edit.
func (this *GedView) setSelectionLockPos(lock bool) {
	for _, item := range this.Selection().ItemList() {
		item.SetLockPos(lock)
	}
	this.reseatSelection()
}

// setSelectionLockSize pins (or releases) the size of every selected item.
// Not undoable, for the same reason as setSelectionLockPos.
func (this *GedView) setSelectionLockSize(lock bool) {
	for _, item := range this.Selection().ItemList() {
		item.SetLockSize(lock)
	}
	this.reseatSelection()
}

// reseatSelection re-selects the current items so their decors are rebuilt
// against the flags they carry now. Not cosmetic: decors are generated once,
// when an item is selected (graph emitItemSelected -> GenDecors), and GenDecors
// hands a locked item no handles — but an item locked while it was already
// selected keeps the handles it was given, and ResizeDecor.OnEndMoveHandle
// resizes its own item without consulting the lock.
func (this *GedView) reseatSelection() {
	sel := this.Selection()
	items := sel.ItemList()
	sel.Clear()
	sel.AddMulti(items)
	this.Self().Update()
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

// BringSelectionToFront and SendSelectionToBack expose the canvas context
// menu's Z-order path to the host application's 排列 menu. Exported wrappers
// rather than a second implementation in package main: the undo here is an
// index snapshot walked back through Raise/Lower, and a menu that restacked
// items on its own would either skip undo or drift from that inverse.
func (this *GedView) BringSelectionToFront() {
	this.reorderSelection(graph.IItem.BringToFront)
}

func (this *GedView) SendSelectionToBack() {
	this.reorderSelection(graph.IItem.SendToBack)
}

// AlignMode selects an align-or-distribute operation. The first six entries
// reposition every rect onto a shared edge or centre line; AlignCenter centres
// on both axes at once; the last two even out the gaps between rects along one
// axis. Exported because the host application owns the 排列 menu and drives
// these through GedView.AlignSelection.
type AlignMode int

const (
	AlignLeft AlignMode = iota
	AlignRight
	AlignHCenter
	AlignTop
	AlignBottom
	AlignVCenter
	AlignCenter
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
//
// clamped reports that a distribute hit the minimum-gap guard (see
// distributeAxis) — the caller says so on the status bar, because a distribute
// that could not honour the requested span is not the result the user asked
// for. It is always false for the align modes.
//
// AlignCenter is absent from the switch on purpose: centring has no meaning
// against the selection's own extremes (it would pile every rect onto one
// point). It is a frame-relative operation only — see frameAlignDelta.
func alignRects(rects []geom.Rect, mode AlignMode) (out []geom.Rect, clamped bool) {
	out = make([]geom.Rect, len(rects))
	copy(out, rects)
	if len(rects) < 2 {
		return out, false
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
		clamped = distributeAxis(out, true)

	case DistributeV:
		clamped = distributeAxis(out, false)
	}

	return out, clamped
}

// distributeAxis evens out the gaps between rects along one axis (horizontal
// when horizontal is true, vertical otherwise). The leftmost/topmost and
// rightmost/bottommost rects stay put; the interior rects are slid so every
// consecutive gap is identical. Operates in place on out. No-op for <3 rects.
//
// Returns true when the minimum-gap guard fired: rects whose combined extent
// exceeds the span between the outer two yield a NEGATIVE even gap, which slid
// the interior rects backwards over their neighbours — a "distribute" that
// stacked widgets on top of each other. The gap is clamped at zero instead and
// the rects pack edge to edge from the first one, which means the trailing rect
// has to move as well: leaving it anchored is exactly what the rect before it
// would run over.
func distributeAxis(out []geom.Rect, horizontal bool) bool {
	n := len(out)
	if n < 3 {
		return false
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
	clamped := gap < 0
	end := n - 1
	if clamped {
		gap = 0
		end = n
	}

	cursor := lead(first) + extent(first)
	for k := 1; k < end; k++ {
		i := idx[k]
		pos := cursor + gap
		if horizontal {
			out[i].X = pos
		} else {
			out[i].Y = pos
		}
		cursor = pos + extent(i)
	}
	return clamped
}

// frameRect returns parent's content rect expressed in the coordinate space
// ITS CHILDREN's bounds use.
//
// A parent that hands its children a local coordinate system — graph.SceneItem,
// i.e. the form itself — measures them from its own top-left, so its frame
// starts at the origin whatever the form's own position is. Every other parent
// (a container FakeWidget: VBox, GroupBox, Card, …) shares its own space with
// its children, so the frame is simply its bounds. The designer places children
// at raw coordinates with no inset — GedView.OnDrop parents a widget into the
// container under the cursor without translating it — so a container's content
// rect is its whole rect.
func frameRect(parent graph.IItem) geom.Rect {
	if parent.HasLocalCoord() {
		w, h := parent.Size()
		return geom.Rect{Width: w, Height: h}
	}
	return parent.Bounds1()
}

// alignFrame returns the rect the frame-relative operations measure against:
// the content rect of the one parent every selected item shares — the
// container when the selection sits inside one, the form when it sits at the
// root.
//
// ok is false when the selection spans parents. The items' coordinates would
// still be comparable (only the scene sets HasLocalCoord, so the whole design
// shares one space), but the FRAMES would not be: half the selection would be
// pushed against a container's edge and half against the form's. The caller
// refuses instead, and says why.
func alignFrame(items []graph.IItem) (geom.Rect, bool) {
	if len(items) == 0 {
		return geom.Rect{}, false
	}
	parent := items[0].Parent()
	if parent == nil {
		return geom.Rect{}, false
	}
	for _, it := range items[1:] {
		if it.Parent() != parent {
			return geom.Rect{}, false
		}
	}
	return frameRect(parent), true
}

// frameAlignDelta returns the translation that puts box — the bounding box of
// what is selected — against frame for mode. One shared delta, so a group keeps
// its internal layout while landing where the command names.
//
// This is the reference frame the selection cannot provide for itself: a lone
// widget has no min-left but its own, and centring is only meaningful against
// something. The distribute modes have no frame meaning and translate by
// nothing.
func frameAlignDelta(box, frame geom.Rect, mode AlignMode) (dx, dy float64) {
	fcx, fcy := frame.Center()
	bcx, bcy := box.Center()
	switch mode {
	case AlignLeft:
		dx = frame.Left() - box.Left()
	case AlignRight:
		dx = frame.Right() - box.Right()
	case AlignHCenter:
		dx = fcx - bcx
	case AlignTop:
		dy = frame.Top() - box.Top()
	case AlignBottom:
		dy = frame.Bottom() - box.Bottom()
	case AlignVCenter:
		dy = fcy - bcy
	case AlignCenter:
		dx, dy = fcx-bcx, fcy-bcy
	}
	return
}

// AlignSelection repositions the selection for mode as one undoable step.
//
// Two reference frames, and which one applies is decided by the selection, not
// by the mode:
//
//   - Two or more items, edge/centre-line modes: the reference is the
//     selection's own extremes (min-left, max-right, mean-centre, …) — the
//     rule every form designer uses, and the one alignRects implements. The
//     per-item targets go onto one MoveCommand; MoveCommand.AddItem takes
//     absolute (toX, toY), so N distinct targets need no composite and Ctrl+Z
//     snaps everyone back at once. Each target carries the item's own subtree
//     (graph.AddMoveSubtree), so aligning a container aligns what is in it —
//     and a widget already riding along that way is out of the extremes as
//     well as out of the command; see the comment on movers.
//     Position-locked items are left where they are, as on every other move
//     path — but they still count in the extremes, and 分布 refuses outright
//     rather than spread the rest around one; see the two comments below.
//   - 居中 (AlignCenter), and any mode with a single item selected: there are
//     no selection extremes worth aligning to, so the reference is the frame —
//     the container the items live in, or the form when they sit at the root.
//     That move is one shared delta, which is exactly what
//     Selection.GenerateMoveCommand builds (and it skips position-locked and
//     ancestor-selected items on the way, as every other move path does).
//
// Both paths push nothing when nothing would move, so a no-op command never
// lands on the UndoStack.
func (this *GedView) AlignSelection(mode AlignMode) {
	items := this.Selection().ItemList()
	if len(items) == 0 {
		return
	}
	rects := make([]geom.Rect, len(items))
	for i, it := range items {
		rects[i] = it.Bounds1()
	}

	if mode == AlignCenter || len(items) == 1 {
		frame, ok := alignFrame(items)
		if !ok {
			this.showStatus("对齐：所选控件没有共同的容器，已跳过")
			return
		}
		dx, dy := frameAlignDelta(boundingBoxOf(rects), frame, mode)
		cmd := this.Selection().GenerateMoveCommand(dx, dy)
		if cmd == nil {
			return
		}
		this.Scene().PushCommand(cmd)
		this.Self().Update()
		return
	}

	selSet := make(map[graph.IItem]bool, len(items))
	for _, it := range items {
		selSet[it] = true
	}

	// The geometry is measured over the items that will actually move, so a
	// widget inside a selected container drops out here rather than at the
	// command below. It never gets a target of its own — it rides with the
	// container (graph.AddMoveSubtree) — so its rect is not an edge anything
	// can align to: it travels by the container's delta whatever alignRects
	// says about it. Counting it computed the answer for N items and applied it
	// to N-1, which is how a 水平分布 over a container, a widget inside it and
	// two others came out spaced for four things and moved three.
	//
	// This is the opposite of the rule position locks get, and the difference
	// is whether the rect moves: a locked item STAYS, so its edge is a real
	// fixed edge and making the group align to it is the point of pinning it
	// (see the two comments below). A carried item's edge leaves with its
	// container. That holds for the align modes as much as for 分布 — the mean
	// of AlignHCenter is a balance point over the widgets that swing to it, and
	// a child hanging over its container's left edge would otherwise set the
	// AlignLeft line and then walk off it.
	movers := make([]graph.IItem, 0, len(items))
	moverRects := make([]geom.Rect, 0, len(items))
	for i, it := range items {
		if hasSelectedAncestor(it, selSet) {
			continue
		}
		movers = append(movers, it)
		moverRects = append(moverRects, rects[i])
	}
	aligned, clamped := alignRects(moverRects, mode)

	// 分布 is one joint constraint, not N independent targets: the gaps only
	// come out even if every item takes the slot distributeAxis gave it. A
	// position-locked item keeps the place it had, so the others would land on
	// a lattice measured around a widget that never moved and not one gap
	// would be the gap that was asked for. There is no partial answer worth
	// shipping, so the whole command is refused and the reason is said out
	// loud. A lock on one of the two outer items costs nothing — those are the
	// anchors and stay put by construction — hence the target-differs test
	// rather than a bare IsLockPos.
	//
	// Only the movers are asked: an item carried by its selected container
	// moves whatever its own lock says (graph.AddMoveSubtree), so vetoing the
	// command over that lock would refuse a 分布 nothing was blocking.
	if mode == DistributeH || mode == DistributeV {
		for i, it := range movers {
			if !it.IsLockPos() {
				continue
			}
			if x, y := it.Pos(); x != aligned[i].X || y != aligned[i].Y {
				if mode == DistributeH {
					this.showStatus("水平分布：所选控件中有位置锁定的控件，间距无法均分，已跳过")
				} else {
					this.showStatus("垂直分布：所选控件中有位置锁定的控件，间距无法均分，已跳过")
				}
				return
			}
		}
	}

	cmd := graph.NewMoveCommand()
	for i, it := range movers {
		// 位置锁定 already makes a drag and an arrow key leave the widget alone
		// (GenerateMoveCommand, snapSelectionToGrid); 对齐 is a move like any
		// other and does too — the lock was never meant to mean "unless you
		// reach it from the align menu".
		// The item still counts in alignRects' geometry above, deliberately:
		// its edge is a real edge of the selection, so locking the extreme
		// widget makes it the line everyone else aligns to — which is what
		// pinning it was for — and dropping it from the geometry would instead
		// send the group sailing past it.
		if it.IsLockPos() {
			continue
		}
		oldX, oldY := it.Pos()
		if oldX == aligned[i].X && oldY == aligned[i].Y {
			continue
		}
		// Per-item targets, so the shared delta of GenerateMoveCommand does not
		// apply — but the subtree still has to come along.
		graph.AddMoveSubtree(cmd, it, aligned[i].X-oldX, aligned[i].Y-oldY)
	}
	if cmd.Count() == 0 {
		return
	}
	this.Scene().PushCommand(cmd)
	// The clamp changes what the command means — the widgets are packed, not
	// spread — so it is reported rather than left to be discovered.
	if clamped {
		if mode == DistributeH {
			this.showStatus("水平分布：控件总宽超出可分布范围，间距已压缩为 0")
		} else {
			this.showStatus("垂直分布：控件总高超出可分布范围，间距已压缩为 0")
		}
	}
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

// renameTarget returns the widget F2 should rename: exactly one selected
// FakeWidget. A multi-selection has no single name to edit, and only a
// FakeWidget carries a widget name at all.
func (this *GedView) renameTarget() *FakeWidget {
	items := this.Selection().ItemList()
	if len(items) != 1 {
		return nil
	}
	fake, _ := items[0].(*FakeWidget)
	return fake
}

// showInputBox opens the name prompt. Indirected through a package var so the
// rename path can be driven headlessly — gui.ShowInputBox runs a modal loop that
// needs a window. Tests swap it; nothing else may reassign it.
var showInputBox = gui.ShowInputBox

// renameItem shows an input dialog to set the widget name of a FakeWidget.
func (this *GedView) renameItem(item graph.IItem) {
	if fake, ok := item.(*FakeWidget); ok {
		name, ok := showInputBox(this, nil, "设置名称", "控件名称:", fake.WidgetName())
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

	// A press on a ruler, or on a guide line, is a guide gesture — the canvas
	// tools must not also see it as a click on whatever sits underneath.
	if this.beginGuideDrag(x, y) {
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

	// Both are (re)decided by OnMouseMove; a press that is never followed by
	// one is a selection click and must leave the tree alone.
	this.dropContainer = nil
	this.dragMovesItems = false
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

// keyDown probes live modifier state. Indirected through a package var because
// gui.IsKeyDown polls the GLFW window: with no window there is no way to hold a
// modifier, so every Ctrl/Alt/Shift binding below was untestable. Tests swap it
// to drive those bindings; nothing else may reassign it.
var keyDown = gui.IsKeyDown

// OnKeyDown handles keyboard shortcuts for the GED editor.
func (this *GedView) OnKeyDown(key int, repeat bool) {
	ctrl := keyDown(gui.KeyCtrl)
	alt := keyDown(gui.KeyMenu)

	// Space key enables pan mode
	if key == gui.KeySpace && !ctrl && !alt {
		this.spacePanReady = true
		return
	}

	switch {
	// Tab / Shift+Tab: cycle widget selection
	case key == gui.KeyTab:
		if keyDown(gui.KeyShift) {
			this.selectPrevWidget()
		} else {
			this.selectNextWidget()
		}

	case key == gui.KeyDelete || key == gui.KeyBackSpace:
		this.DeleteSelectedItems()

	// F2 renames the selected widget. Every other way to reach the name — the
	// right-click menu, the property sheet — needs the mouse, which left the
	// widget name as the one property a keyboard-only user could not edit. It
	// reuses the context menu's prompt rather than adding a second name editor.
	case key == gui.KeyF2:
		if target := this.renameTarget(); target != nil {
			this.renameItem(target)
		}

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

	// Ctrl+arrow resizes the selection instead of moving it: Right/Down grow,
	// Left/Up shrink, with each item's top-left corner pinned so only the size
	// changes. Reads the same `ctrl` probe as every other shortcut in this
	// switch, and must precede the plain-arrow case below because both match
	// the same four keys.
	case ctrl && (key == gui.KeyLeft || key == gui.KeyRight || key == gui.KeyUp || key == gui.KeyDown):
		if this.Selection().IsEmpty() {
			return
		}
		this.beginGesture(repeat)
		// The arrow→delta mapping is identical for moving and resizing, so the
		// nudge helper does double duty here rather than being copied.
		dw, dh, _ := nudgeDelta(key, keyDown(gui.KeyShift), 1, this.coarseNudgeStep())
		this.resizeSelection(dw, dh)

	// Arrow keys nudge the selection by 1mm (one grid cell with Shift).
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
		this.beginGesture(repeat)
		dx, dy, _ := nudgeDelta(key, keyDown(gui.KeyShift), 1, this.coarseNudgeStep())
		this.nudgeSelection(dx, dy)

	case ctrl && (key == 'Z' || key == 'z'):
		this.Undo()

	case ctrl && (key == 'Y' || key == 'y'):
		this.Redo()

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

	// Cmd+D / Ctrl+D: duplicate the selection in place. Designer-tool muscle
	// memory: Figma, JetBrains, and Sketch all bind Cmd+D this way. Shares the
	// entry point with the 编辑 → 创建副本 menu command.
	case ctrl && (key == 'D' || key == 'd'):
		this.DuplicateSelection()

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

// readingOrder returns items sorted by Y then X (top-to-bottom, left-to-right)
// without touching the input slice.
func readingOrder(items []graph.IItem) []graph.IItem {
	sorted := make([]graph.IItem, len(items))
	copy(sorted, items)
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

// sortedWidgetList returns every keyboard-reachable widget in tab order: scene
// children in reading order, each container immediately followed by its own
// children in the same order.
//
// The walk must recurse. It used to list only the scene's direct children, so a
// widget dropped into a container could be selected with the mouse and by
// Ctrl+A — which does recurse — but never by Tab, leaving nested widgets
// unreachable for a keyboard-only user. Hidden and unselectable items are
// skipped for the same reason selectAll skips them: Tab must not park the
// selection on something the canvas will not draw handles for.
func (this *GedView) sortedWidgetList() []graph.IItem {
	var walk func(items []graph.IItem) []graph.IItem
	walk = func(items []graph.IItem) []graph.IItem {
		var out []graph.IItem
		for _, item := range readingOrder(items) {
			// A hidden container hides its children too, so the whole subtree
			// drops out of the walk.
			if !item.IsVisible() {
				continue
			}
			if item.IsSelectable() {
				out = append(out, item)
			}
			if item.HasChildren() {
				out = append(out, walk(item.Children())...)
			}
		}
		return out
	}
	return walk(this.Scene().Children())
}

// WantsTab claims Tab for the canvas so it walks the widgets being designed.
//
// Without this the key never arrives: gui's window layer owns Tab for focus
// traversal and spends it before OnKeyDown runs, moving focus to the next IDE
// panel and taking it off the canvas — so the next Tab walks further away and
// the widget is unreachable from the keyboard, which is the whole point of the
// binding. Claimed only when the canvas has something to walk, so an empty
// form still lets Tab leave the panel.
func (this *GedView) WantsTab(shift bool) bool {
	if len(this.sortedWidgetList()) == 0 {
		return false
	}
	if shift {
		this.selectPrevWidget()
	} else {
		this.selectNextWidget()
	}
	return true
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

// beginGesture opens a new merge group unless this key event is an
// auto-repeat. Commands pushed under the same gesture number collapse into a
// single undo step (graph.MoveCommand.MergeWidth), so holding an arrow key
// down leaves one entry on the stack instead of one per repeat — a held key
// used to bury the previous edit under hundreds of one-millimetre moves.
// Releasing and pressing again opens a new group, and so a new undo step.
func (this *GedView) beginGesture(repeat bool) {
	if !repeat {
		this.gestureSeq++
	}
}

// minWidgetSize is the floor (in mm) a keyboard resize will not shrink past.
const minWidgetSize = 1.0

// coarseNudgeStep is the larger Shift+arrow move/resize step (in mm). Qt
// Designer and every IDE bind plain arrows to a 1-unit "fine move" and
// Shift+arrow to a coarse grid-sized jump, so this reads the scene's grid
// pitch: it used to be a 10mm constant that ignored the grid the designer had
// actually configured.
func (this *GedView) coarseNudgeStep() float64 {
	return this.gridModel().Pitch
}

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
// Note on undo: unlike AlignSelection / reorderSelection (which mutate +
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
	cmd.SetMergeToken(this.gestureSeq)
	this.Scene().PushCommand(cmd)
	this.Self().Update()
}

// resizeSelection grows every selected item by (dw, dh) millimetres, anchored
// at the top-left corner so nothing moves. Mirrors nudgeSelection: one
// ResizeCommand on the UndoStack, tagged with the current gesture so a held
// Ctrl+arrow collapses into a single undo step.
//
// Quiet no-op when the selection is empty, entirely size-locked, or already at
// or under minWidgetSize in the requested direction — GenerateResizeCommand
// returns nil for all three, which we forward rather than pushing an empty
// command.
func (this *GedView) resizeSelection(dw, dh float64) {
	cmd := this.Selection().GenerateResizeCommand(dw, dh, minWidgetSize)
	if cmd == nil {
		return
	}
	cmd.SetMergeToken(this.gestureSeq)
	this.Scene().PushCommand(cmd)
	this.Self().Update()
}

// SameSizeSelection resizes every selected item onto the last-selected item's
// width and/or height, keeping each item's position. Exported because the host
// application owns the 排列 menu but not minWidgetSize — routing the menu back
// through here keeps one floor value behind every resize path instead of a
// second copy in package main.
//
// One press is one undo step: no merge token, because a menu command is not a
// repeating gesture the way a held Ctrl+arrow is.
func (this *GedView) SameSizeSelection(mode graph.SameSizeMode) {
	cmd := this.Selection().GenerateSameSizeCommand(mode, minWidgetSize)
	if cmd == nil {
		return
	}
	this.Scene().PushCommand(cmd)
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

// CopySelected serializes every selected FakeWidget into the clipboard through
// the same SaveDesign the file format uses, so a copy carries the whole widget:
// its properties, its event bindings, its handler code and its children.
//
// A selected widget whose container is also selected is left out: it already
// rides inside the container's subtree, and copying it as a second root would
// paste it twice — once nested, once loose on the canvas.
func (this *GedView) CopySelected() {
	clipboard = nil
	items := this.Selection().ItemList()
	selSet := make(map[graph.IItem]bool, len(items))
	for _, item := range items {
		if _, ok := item.(*FakeWidget); ok {
			selSet[item] = true
		}
	}
	for _, item := range items {
		fake, ok := item.(*FakeWidget)
		if !ok || hasSelectedAncestor(item, selSet) {
			continue
		}
		clipboard = append(clipboard, fake.SaveDesign())
	}
}

// PasteItems rebuilds each clipboard subtree through the design loader and
// drops it on the canvas offset by one grid cell, so the copies don't sit on
// top of the originals after snap-rounding. With the default 5mm grid, a fixed
// (2, 2) nudge plus snap actually rounded BACK to the original cell — the
// per-cell step here keeps the offset visible whatever the grid.
func (this *GedView) PasteItems() {
	if len(clipboard) == 0 {
		return
	}
	step := this.gridModel().Pitch
	if step <= 0 {
		step = defaultGridPitch
	}
	// Track names already present in the scene plus names handed out
	// earlier in this same paste so a multi-item paste doesn't collide
	// with itself.
	taken := this.sceneWidgetNames()
	sel := this.Selection()
	sel.Clear()
	// A paste is one gesture, so the whole clipboard rides in one AddCommand:
	// a command per root would cost one Ctrl+Z per pasted widget to take back.
	cmd := graph.NewAddCommand()
	var pasted []graph.IItem
	for _, doc := range clipboard {
		var factoryName string
		doc.Value(&factoryName)
		item, err := NewFakeWidgetFromFactory(factoryName)
		if err != nil {
			continue
		}
		// Deserializing through LoadDesign is what restores the geometry,
		// properties, event bindings and children the entry carries.
		item.LoadDesign(doc)
		px, py := this.snapToGrid(item.X()+step, item.Y()+step)
		offsetSubtree(item, px, py)
		renameSubtreeUnique(item, taken)
		item.Layout()

		cmd.AddItem(item, this.Scene())
		pasted = append(pasted, item)
	}
	// A paste whose every factory lookup failed changed nothing: pushing the
	// empty command would spend an undo step and report a design change.
	if cmd.Count() > 0 {
		this.Scene().PushCommand(cmd)
	}
	// Selection comes after the push so the items are attached to the scene by
	// the time anyone listening on the selection goes looking for them.
	for _, item := range pasted {
		sel.Add(item)
	}
	this.Self().Update()
}

// DuplicateSelection copies the selection and pastes it straight back, the
// one-step Ctrl+D / 编辑 → 创建副本 command. It is the copy/paste pipeline
// rather than a second one beside it: PasteItems already offsets each copy by
// one grid pitch, hands it a unique name and leaves the selection on the new
// items. Like every Cmd+D built this way it overwrites the clipboard — the
// price of not carrying a second serializer.
func (this *GedView) DuplicateSelection() {
	this.CopySelected()
	this.PasteItems()
}

// sceneWidgetNames returns the set of non-empty widget names currently
// present in the scene. Used to keep pasted/duplicated widget names
// unique. It walks the whole tree, not just the scene's own children:
// codegen declares one struct field per widget wherever it sits, so a name
// nested inside a container collides just as hard as a top-level one.
func (this *GedView) sceneWidgetNames() map[string]bool {
	taken := map[string]bool{}
	var walk func(items []graph.IItem)
	walk = func(items []graph.IItem) {
		for _, item := range items {
			if fake, ok := item.(*FakeWidget); ok {
				if n := fake.WidgetName(); n != "" {
					taken[n] = true
				}
			}
			walk(item.Children())
		}
	}
	walk(this.Scene().Children())
	return taken
}

// renameSubtreeUnique hands every FakeWidget in a pasted subtree a name that is
// free in the document, recording each one in taken so the copy's own children
// cannot collide with each other either. Same uniquifier the roots have always
// used — the subtree just extends its reach.
func renameSubtreeUnique(root *FakeWidget, taken map[string]bool) {
	name := uniqueWidgetName(root.WidgetName(), func(n string) bool { return taken[n] })
	if name != "" {
		taken[name] = true
	}
	root.SetWidgetName(name)
	for _, c := range root.Children() {
		if fake, ok := c.(*FakeWidget); ok {
			renameSubtreeUnique(fake, taken)
		}
	}
}

// offsetSubtree moves item to (x, y) and shifts everything below it by the same
// delta. Child bounds are scene coordinates — ged never turns on local coords,
// so moving a container does not carry its children (graph.Item.SetPos) — and
// offsetting the container alone would leave the copied children sitting
// exactly on top of the originals', which is the collision the paste offset
// exists to prevent.
func offsetSubtree(item graph.IItem, x, y float64) {
	ox, oy := item.Pos()
	dx, dy := x-ox, y-oy
	item.SetPos(x, y)
	var shift func(items []graph.IItem)
	shift = func(items []graph.IItem) {
		for _, c := range items {
			cx, cy := c.Pos()
			c.SetPos(cx+dx, cy+dy)
			shift(c.Children())
		}
	}
	shift(item.Children())
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

// Close is what the dock calls as this document's tab goes away, and the only
// notice the host application gets that a design is no longer open. Both close
// paths look for it: Dock.CloseIndex and gui.PromptSaveClose each type-assert
// the view for a Close() method, so the tab's × and 文件/关闭 arrive here alike.
//
// Anything the host keyed on the document — the 问题 rows filed against it, for
// one — outlives the design without this, and goes on describing a canvas the
// user cannot open, let alone fix.
func (this *GedView) Close() {
	if DocClosedCallback != nil {
		DocClosedCallback(this)
	}
}

// ---------------------------------------------------------------------------
// Alignment guide system (Qt Creator-style snap lines during drag)
// ---------------------------------------------------------------------------

const alignTolerance = 1.5 // mm — snap distance for alignment guides

// computeAlignGuides is the pure geometry behind the drag-time alignment
// guides. It returns one guide for every case where the dragged rect's left,
// right, or horizontal centre lines up — within threshold — with the left,
// right, or centre of any rect in others, with one of the user's manual
// guides, or with the canvas horizontal centre (and the top/bottom/centre
// analogue for horizontal guides). Vertical guides are emitted as a segment
// spanning the canvas height at the aligned x; horizontal guides span the
// canvas width at the aligned y. Receiver- and scene-free (mirrors the pure
// snapToGrid) so it is unit-testable in isolation.
func computeAlignGuides(dragged geom.Rect, others []geom.Rect, canvas geom.Rect, manual sceneGuides, threshold float64) []alignGuide {
	var guides []alignGuide

	dcx, dcy := dragged.Center()
	xEdges := [3]float64{dragged.Left(), dragged.Right(), dcx}
	yEdges := [3]float64{dragged.Top(), dragged.Bottom(), dcy}

	top, bottom := canvas.Top(), canvas.Bottom()
	left, right := canvas.Left(), canvas.Right()
	canCx, canCy := canvas.Center()

	// A vertical guide runs down the canvas at the target x; a horizontal one
	// runs across it at the target y. Snap to the target coordinate so guides
	// that share an edge coincide exactly.
	tryV := func(edge, target float64) {
		if math.Abs(edge-target) < threshold {
			guides = append(guides, alignGuide{x1: target, y1: top, x2: target, y2: bottom, snap: true})
		}
	}
	tryH := func(edge, target float64) {
		if math.Abs(edge-target) < threshold {
			guides = append(guides, alignGuide{x1: left, y1: target, x2: right, y2: target, snap: true})
		}
	}

	for _, o := range others {
		ocx, ocy := o.Center()
		xTargets := [3]float64{o.Left(), o.Right(), ocx}
		yTargets := [3]float64{o.Top(), o.Bottom(), ocy}
		for _, e := range xEdges {
			for _, t := range xTargets {
				tryV(e, t)
			}
		}
		for _, e := range yEdges {
			for _, t := range yTargets {
				tryH(e, t)
			}
		}
	}

	// Manual guides are just more targets in the same pass, so a widget lines
	// up against them exactly as it does against a sibling edge.
	for _, e := range xEdges {
		for _, t := range manual.V {
			tryV(e, t)
		}
	}
	for _, e := range yEdges {
		for _, t := range manual.H {
			tryH(e, t)
		}
	}

	// Canvas centre lines — the classic "centre on the page" guide.
	for _, e := range xEdges {
		tryV(e, canCx)
	}
	for _, e := range yEdges {
		tryH(e, canCy)
	}

	return guides
}

// computeAlignGuides (method) refreshes this.alignGuides for the live drag: it
// builds the proposed rect for each selected item (its current position offset
// by dx,dy) and the rects of every non-selected sibling, then delegates to the
// pure computeAlignGuides helper with the full scene as the canvas. Guides are
// purely visual and never move a widget.
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
	canvas := geom.Rect{X: 0, Y: 0, Width: sceneW, Height: sceneH}

	// Build a set of selected items for fast lookup, then collect the rects of
	// every non-selected sibling as alignment targets.
	selSet := make(map[graph.IItem]bool, len(sel))
	for _, s := range sel {
		selSet[s] = true
	}
	var others []geom.Rect
	for _, other := range scene.Children() {
		if selSet[other] {
			continue
		}
		others = append(others, geom.Rect{X: other.X(), Y: other.Y(), Width: other.Width(), Height: other.Height()})
	}

	manual := this.activeGuides()
	for _, dragging := range sel {
		dragged := geom.Rect{
			X:      dragging.X() + dx,
			Y:      dragging.Y() + dy,
			Width:  dragging.Width(),
			Height: dragging.Height(),
		}
		this.alignGuides = append(this.alignGuides, computeAlignGuides(dragged, others, canvas, manual, alignTolerance)...)
	}
}

// drawGrid paints a faint dot at every grid intersection across the page area.
// Called in scene-mm coordinate space (same transform as drawAlignGuides), so
// the dot spacing tracks the pan/zoom the canvas already applied. Dots are
// drawn with a tiny zero-width-pen cross per intersection rather than filled
// circles to stay cheap and crisp at any zoom; only the visible page rect is
// walked so the loop count scales with page size / grid pitch, not the canvas.
func (this *GedView) drawGrid(g paint.Painter) {
	step := this.gridModel().Pitch
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

// Draw overrides GraphView.Draw to add the background grid, the manual guide
// lines, the alignment guide overlay and the rulers. The grid (when enabled)
// is painted first as a faint backdrop hint; the guides are painted over it,
// and the rulers last so nothing draws across them.
func (this *GedView) Draw(g paint.Painter) {
	this.GraphView.Draw(g)

	// The scene's visibility flag is the master switch for every grid the
	// canvas paints; showGrid is this overlay's own extra opt-in on top of it.
	grid := this.gridModel()
	wantGrid := this.showGrid && grid.Visible && grid.Pitch > 0
	manual := this.activeGuides()
	wantGuides := !manual.isEmpty() || this.guideDrag.active
	if wantGrid || wantGuides || len(this.alignGuides) > 0 || this.dropContainer != nil {
		// Clip to the page area so neither the grid nor the guides bleed into
		// the padding region, then re-enter scene-mm coordinate space (the
		// same transform GraphView.Draw uses for the scene) so every overlay
		// honours the current pan/zoom.
		g.Save()
		// Margins are hidden on the designer canvas, so the page rect is the
		// scene rect; asking for the margin-inclusive size here would clip a
		// margin's worth of padding region in as well.
		pw, ph := this.PageSizePx(this.IsPageMarginVisible(), this.ZoomFactor())
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
		if wantGuides {
			this.drawGuides(g, manual)
		}
		this.drawAlignGuides(g)
		this.drawDropContainer(g)
		g.Restore()
	}

	if this.showGuides {
		this.drawRulers(g)
	}
}

// drawDropContainer outlines the container the current drag would nest into.
// Called in scene-mm coordinate space; the item's CoordOffset is applied first
// so a container living in a local-coordinate parent is outlined where it is
// drawn — the same shift Selection.OnDraw applies to its decorations.
//
// The palette drop path has no highlight to reuse (OnDragMove only sets the
// drag action), so this is the design's fallback accent border. Its width is in
// millimetres, the only thing this painter's scene space can express: 0.4mm
// reads as a target next to the 0.2mm border FakeWidget draws around itself,
// where a zero-width pen would collapse into the same hairline as the selection
// outline.
func (this *GedView) drawDropContainer(g paint.Painter) {
	if this.dropContainer == nil {
		return
	}
	dx, dy := this.dropContainer.CoordOffset()
	g.Translate(dx, dy)
	g.SetPen1(gui.Theme().HighLightColor, 0.4)
	g.Rectangle(this.dropContainer.Bounds())
	g.Stroke()
	g.Translate(-dx, -dy)
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

	if this.updateGuideDrag(x, y) {
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
		this.dropContainer = nil
		return
	}

	sx, sy := this.MapToScene(x, y)
	dx := sx - this.dragOriginX
	dy := sy - this.dragOriginY

	this.computeAlignGuides(sel.ItemList(), dx, dy)

	// A gesture only re-parents when it is a widget drag: the press must have
	// landed on a movable item — MovePart's own claim test, so a marquee never
	// re-parents — and the pointer must have moved at all, which is what being
	// here proves.
	//
	// The drop target follows the cursor, not the dragged rects: the widgets
	// stay at their old positions until MovePart commits on release.
	this.dropContainer = nil
	// Ask who claimed the press, in the order the parts themselves are asked:
	// DecorPart pre-empts MovePart, and a resize handle's disc reaches inside
	// the item it belongs to, so hit-testing the item alone reads a resize that
	// happens to end over a container as a widget move — and silently re-parents
	// the widget mid-resize, undo step and drop highlight included.
	if decor, _ := this.FindHandleAt(this.dragOriginX, this.dragOriginY); decor != nil {
		this.dragMovesItems = false
		return
	}
	this.dragMovesItems = this.Scene().FindItemAt(this.dragOriginX, this.dragOriginY,
		graph.TraversalCond_SelectableAndMoveable) != nil
	if this.dragMovesItems {
		this.dropContainer = nearestDropContainer(
			this.Scene().FindItemAt(sx, sy, nil), sel.Contains)
	}
}

// OnLeftUp overrides GraphView.OnLeftUp to clear alignment guides when
// the drag ends.
func (this *GedView) OnLeftUp(x, y float64) {
	// End Space+Drag pan
	if this.isPanning {
		this.isPanning = false
		return
	}

	if this.endGuideDrag(x, y) {
		return
	}

	wasDragging := this.isDragging
	// The target and the gesture kind are read before GraphView.OnLeftUp so a
	// tool that clears the drag state cannot take them away from us.
	target, movesItems := this.dropContainer, this.dragMovesItems
	this.GraphView.OnLeftUp(x, y)
	this.isDragging = false
	this.alignGuides = nil
	this.dropContainer = nil
	this.dragMovesItems = false

	// Snap the dragged widget(s) onto grid intersections once the base
	// MovePart has committed the move command. We snap after the commit
	// (rather than rewriting the delta mid-drag, which lives in the graph
	// MovePart we don't own) so the final resting position lands on the
	// grid. The align guides drawn during the drag are purely visual and
	// don't move the widget, so grid snap and the guides compose cleanly.
	if wasDragging {
		this.snapSelectionToGrid()
		if movesItems {
			// After the snap, so the re-parent records the resting position —
			// and so the park judges that same position. target was chosen
			// from the cursor mid-drag, before the snap existed; the snap then
			// moves the widget by up to half a grid pitch (a manual guide,
			// further still), which is enough to carry a widget released near
			// a container's edge clear of the container it is about to be
			// parented into. The park follows the re-parent rather than
			// preceding it — parkInsideParent measures against the new parent
			// and only sees items already owned by it — and rides its undo
			// step, because between the two the widget is owned by a container
			// it does not overlap and DrawAll erases it.
			this.reparentSelection(target, &parkStep{
				items:  this.Selection().ItemList(),
				parent: target,
			})
		}
	}

	this.Self().Update()
}

// reparentSelection nests the just-dragged selection into target, nil meaning
// the form root — a container is only one case of a drop target, the same way
// OnDrop falls back to the scene root for a palette drop over empty canvas.
//
// One ReparentCommand carries parent, sibling index and position for every
// item, so Ctrl+Z returns each one to the owner and slot it left. The move
// itself is MovePart's own command underneath, so a drag that crosses into a
// container leaves two undo steps: the re-parent, then the move.
//
// park is the rescue both drops need — the tree's because it has no pointer at
// all, the canvas's because the grid snap displaces the widget after the drop
// target was chosen. It is applied and undone with the re-parent as one step,
// never as a step of its own: between the two the widget is owned by a
// container it does not overlap, and a widget in that state is invisible. A nil
// park is the caller that has no rescue to tie on.
func (this *GedView) reparentSelection(target graph.IItem, park *parkStep) {
	parent := graph.IItem(this.Scene())
	if target != nil {
		parent = target
	}
	sel := this.Selection()
	items := sel.ItemList()
	cmd := sel.GenerateReparentCommand(parent)
	if cmd == nil {
		return
	}
	this.Scene().PushCommand(groupWithPark(cmd, park))

	// Every re-parent detaches first, and GraphView.emitItemDetached drops a
	// detached item from the selection — so the widget the user just dragged
	// would end the gesture unselected. Restore the selection in its original
	// order (the last item stays the alignment reference), the way
	// layoutSelection re-selects after its own structural command.
	sel.Clear()
	sel.AddMulti(items)
}

// groupWithPark ties the rescue to the re-parent that made it necessary, so the
// pair is one entry on the undo stack. Nothing to tie means the command travels
// on its own.
func groupWithPark(cmd *graph.ReparentCommand, park *parkStep) gui.ICommand {
	if park == nil {
		return cmd
	}
	group := gui.NewCompoundCommand()
	group.SetText(cmd.Text())
	group.Append(cmd)
	group.Append(park)
	return group
}

// snapSelectionToGrid parks every position-unlocked selected item on the
// nearest manual guide, falling back per axis to the nearest grid
// intersection. A guide wins because the user placed it deliberately; without
// that precedence the grid would immediately drag the widget back off a guide
// that does not sit on a grid line. No-op when snap is disabled. Applied
// directly (mutate + Update) like AlignSelection / reorderSelection — it tidies
// the post-drag resting place rather than introducing its own move command.
//
// A correction is a move like any other, so it carries the item's subtree: the
// drag delta is arbitrary but the snapped position is not, so a container drag
// almost always ends with a correction, and without the subtree it would undo
// the carrying the move command just did.
func (this *GedView) snapSelectionToGrid() {
	// grid.Snap is the one master switch for drop/nudge snapping; manual
	// guides ride under it too, so 表单设置 turns every snap off in one place.
	grid := this.gridModel()
	if !grid.Snap {
		return
	}
	guides := this.activeGuides()
	if grid.Pitch <= 0 && guides.isEmpty() {
		return
	}
	items := this.Selection().ItemList()
	selSet := make(map[graph.IItem]bool, len(items))
	for _, item := range items {
		selSet[item] = true
	}
	for _, item := range items {
		if item.IsLockPos() {
			continue
		}
		// The correction below carries the subtree, so an item whose own
		// container is selected too is already being moved by it — snapping it
		// again would shift it twice. Same skip GenerateMoveCommand makes.
		if hasSelectedAncestor(item, selSet) {
			continue
		}
		x, y := item.Pos()
		nx, ny := x, y
		if d, ok := snapOffsetToGuides(x, x+item.Width(), guides.V, alignTolerance); ok {
			nx = x + d
		} else {
			nx = snapToGrid(x, grid.Pitch)
		}
		if d, ok := snapOffsetToGuides(y, y+item.Height(), guides.H, alignTolerance); ok {
			ny = y + d
		} else {
			ny = snapToGrid(y, grid.Pitch)
		}
		if nx != x || ny != y {
			// The move command that put the item here already carried its
			// children; the snap correction has to carry them too, or the
			// widgets inside a dragged container end up offset from it by
			// whatever the rounding was worth.
			graph.ShiftSubtree(item, nx-x, ny-y)
		}
	}
}
