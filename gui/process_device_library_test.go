package gui

import (
	"context"
	"errors"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/paint"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func libraryPane(t *testing.T) (*ProcessEditor, *processDeviceLibraryPane) {
	t.Helper()
	e := NewProcessEditor()
	p, err := newProcessDeviceLibraryPane(e)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { p.Close(); e.Close(); drainUITasks() })
	return e, p
}
func waitLibrary(t *testing.T, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !done() && time.Now().Before(deadline) {
		drainUITasks()
		time.Sleep(time.Millisecond)
	}
	drainUITasks()
	if !done() {
		t.Fatal("library operation did not settle")
	}
}
func TestProcessDeviceLibraryCapturePlacementSingleUndoAndCopyOwnership(t *testing.T) {
	e, p := libraryPane(t)
	before := e.ProjectJSON()
	lib, err := e.DeviceLibrary()
	if err != nil || len(lib.Devices) != 3 || p.capture.IsEnabled() {
		t.Fatal("invalid default library", err)
	}
	lib.Devices[0].Project.Nodes[0].Label = "must not alias"
	copy, _ := e.DeviceLibrary()
	if copy.Devices[0].Project.Nodes[0].Label == "must not alias" {
		t.Fatal("library getter aliases state")
	}
	e.Diagram.SelectNodes([]string{"reactor", "exchanger"})
	p.describe()
	if !p.capture.IsEnabled() {
		t.Fatal("selected subgraph cannot be captured")
	}
	input := processLibraryCaptureInput
	t.Cleanup(func() { processLibraryCaptureInput = input })
	processLibraryCaptureInput = func(IWidget, paint.Icon, string, string, string) (string, bool) { return "custom-recovery", true }
	p.captureSelection()
	if e.ProjectJSON() != before || len(e.deviceLibrary.Devices) != 4 || p.selected != "custom-recovery" {
		t.Fatal("capture changed document or lost template", p.selected)
	}
	p.fields[0].SetText("train1")
	p.fields[1].SetText("Line1.")
	p.fields[2].SetText("300")
	p.fields[3].SetText("300")
	p.search.SetText("custom")
	if p.fields[0].Text() != "train1" {
		t.Fatal("filter reset edited instance ID")
	}
	p.place()
	q := e.Diagram.Project()
	if len(q.Nodes) != 6 || len(q.Links) != 3 || len(q.Points) != 3 || q.Nodes[4].ID != "train1.reactor" || q.Nodes[4].Tag != "Line1.DI11" || q.Nodes[4].X != 300 || q.Nodes[5].Y != 330 || len(e.Diagram.SelectedNodes()) != 2 {
		t.Fatal("GUI placement did not publish complete subgraph", q)
	}
	e.Diagram.Undo()
	if e.ProjectJSON() != before || e.Diagram.CanUndo() {
		t.Fatal("placement was not one undo step")
	}
	e.Diagram.Redo()
	if len(e.Diagram.Project().Nodes) != 6 {
		t.Fatal("redo failed")
	}
}
func TestProcessDeviceLibraryRejectsStaleReadOnlyAndInvalidCatalog(t *testing.T) {
	e, p := libraryPane(t)
	before := e.DeviceLibraryJSON()
	if err := e.SetDeviceLibrary(hmi.DeviceLibrary{Version: 99}); err == nil || e.DeviceLibraryJSON() != before {
		t.Fatal("invalid catalog replaced good state")
	}
	e.SetDeviceLibraryJSON(`{"version":1,"devices":[],"typo":1}`)
	if e.LastError() == nil || e.DeviceLibraryJSON() != before {
		t.Fatal("malformed catalog not rejected")
	}
	p.search.SetText("no-such-device")
	if p.insert.IsEnabled() || p.mapping.IsEnabled() {
		t.Fatal("empty selection left actions enabled")
	}
	p.search.SetText("pump")
	_ = e.Diagram.AddNode("pump")
	source := e.ProjectJSON()
	p.place()
	if e.ProjectJSON() != source || !strings.Contains(p.status.Text(), "document changed") {
		t.Fatal("stale placement accepted")
	}
	e.SetReadOnly(true)
	if _, err := e.Diagram.PlaceDevice(hmi.BuiltinDeviceLibrary().Devices[0], hmi.DevicePlacement{ID: "no"}); err == nil {
		t.Fatal("read-only placement accepted")
	}
}
func TestProcessDeviceLibraryResponsiveOwnedModalAndLockRetirement(t *testing.T) {
	e := NewProcessEditor()
	defer e.Close()
	show := showProcessLibraryDialog
	t.Cleanup(func() { showProcessLibraryDialog = show })
	showProcessLibraryDialog = func(d *Dialog) DialogResult {
		p := d.Content().(*processDeviceLibraryPane)
		if d.Parent() != e || p.Parent() != d || p.preview.IsEditing() {
			t.Fatal("detached library or authoring preview")
		}
		for _, size := range [][2]float64{{620, 480}, {1000, 680}} {
			p.SetSize(size[0], size[1])
			controls := []IWidget{p.search, p.list, p.preview, p.details, p.capture, p.open, p.save, p.mapping, p.insert, p.status}
			for i, c := range controls {
				x, y, w, h := c.Bounds()
				if w <= 0 || h <= 0 || x < 0 || y < 0 || x+w > size[0]+.001 || y+h > size[1]+.001 {
					t.Fatal("control escaped layout", size, i, c.Bounds1())
				}
			}
		}
		e.SetReadOnly(true)
		if !p.closed || !d.closed {
			t.Fatal("lock left library or worker active")
		}
		return DialogCancel
	}
	e.ShowDeviceLibrary()
	if e.closeDeviceLibrary != nil {
		t.Fatal("retired modal retained callback")
	}
}
func TestProcessDeviceLibraryCancelsWorkerAndDropsLateLoad(t *testing.T) {
	e, p := libraryPane(t)
	loader := processLibraryLoad
	t.Cleanup(func() { processLibraryLoad = loader })
	started, ended := make(chan struct{}), make(chan struct{})
	processLibraryLoad = func(ctx context.Context, _ string) (hmi.DeviceLibrary, error) {
		defer close(ended)
		close(started)
		<-ctx.Done()
		return hmi.DeviceLibrary{Version: 1}, nil
	}
	before := e.DeviceLibraryJSON()
	p.load("owned-test")
	<-started
	if !p.busy || p.insert.IsEnabled() || p.save.IsEnabled() {
		t.Fatal("I/O availability not represented")
	}
	p.Close()
	select {
	case <-ended:
	case <-time.After(5 * time.Second):
		t.Fatal("loader not cancelled")
	}
	drainUITasks()
	if e.DeviceLibraryJSON() != before {
		t.Fatal("closed pane applied late catalog")
	}
}
func TestProcessDeviceLibraryAsyncStaleCatalogAndOverwriteConsent(t *testing.T) {
	e, p := libraryPane(t)
	loader, saver, confirm := processLibraryLoad, processLibrarySave, processLibraryConfirmReplace
	t.Cleanup(func() { processLibraryLoad, processLibrarySave, processLibraryConfirmReplace = loader, saver, confirm })
	ready, release := make(chan struct{}), make(chan struct{})
	processLibraryLoad = func(context.Context, string) (hmi.DeviceLibrary, error) {
		close(ready)
		<-release
		return hmi.DeviceLibrary{Version: 1}, nil
	}
	p.load("owned-test")
	<-ready
	_ = e.SetDeviceLibrary(hmi.BuiltinDeviceLibrary())
	close(release)
	waitLibrary(t, func() bool { return !p.busy })
	if len(e.deviceLibrary.Devices) != 3 || !strings.Contains(p.status.Text(), "library changed") {
		t.Fatal("stale catalog replaced newer data")
	}
	var calls atomic.Int32
	processLibrarySave = func(_ context.Context, _ string, _ hmi.DeviceLibrary, overwrite bool) error {
		calls.Add(1)
		if !overwrite {
			return hmi.ErrDeviceLibraryExists
		}
		return nil
	}
	processLibraryConfirmReplace = func(parent IWidget) bool {
		if parent != p {
			t.Fatal("unowned confirmation")
		}
		return true
	}
	p.saveTo("owned-test", false)
	waitLibrary(t, func() bool { return calls.Load() == 2 && !p.busy })
	processLibraryConfirmReplace = func(IWidget) bool { _ = e.SetDeviceLibrary(hmi.BuiltinDeviceLibrary()); return true }
	p.saveTo("owned-test", false)
	waitLibrary(t, func() bool { return calls.Load() >= 3 && !p.busy })
	if calls.Load() != 3 || !strings.Contains(p.status.Text(), "library changed") {
		t.Fatal("confirmation allowed a changed catalog to overwrite")
	}
	processLibrarySave = func(context.Context, string, hmi.DeviceLibrary, bool) error { return errors.New("disk unavailable") }
	p.saveTo("owned-test", false)
	waitLibrary(t, func() bool { return !p.busy })
	if !strings.Contains(p.status.Text(), "disk unavailable") {
		t.Fatal("write failure not surfaced")
	}
}

func TestProcessDevicePointMappingModalOwnershipAndStaleMetadata(t *testing.T) {
	e, p := libraryPane(t)
	hook := showProcessPointMapDialog
	t.Cleanup(func() { showProcessPointMapDialog = hook })
	showProcessPointMapDialog = func(d *Dialog) DialogResult {
		if d.Parent() != p {
			t.Fatal("point mapping detached from library")
		}
		table := d.Content().Children()[0].(*processConfigTable)
		model := table.Model().(*SimpleTableModel)
		model.SetCellText(0, 4, "DI11")
		// Users can sort the table before applying; point identity, not row
		// position, owns the destination mapping.
		model.SortByColumn(0, true)
		d.buttons[1].Action().Trigger(nil)
		if !d.closed || p.pointMap["RUN"] != "DI11" {
			t.Fatal("point mapping not applied")
		}
		return DialogOK
	}
	p.editPointMap()
	showProcessPointMapDialog = func(d *Dialog) DialogResult {
		table := d.Content().Children()[0].(*processConfigTable)
		model := table.Model().(*SimpleTableModel)
		model.SetCellText(0, 1, "float")
		d.buttons[1].Action().Trigger(nil)
		if d.closed {
			t.Fatal("point metadata edit accepted")
		}
		p.Close()
		if !d.closed {
			t.Fatal("closing library left nested mapping dialog floating")
		}
		return DialogCancel
	}
	p.editPointMap()
	if p.pointMap["RUN"] != "DI11" {
		t.Fatal("failed mapping changed previous data")
	}
	_ = e
}
