package hmi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uk0/silk/point"
)

func capturedDevice(t *testing.T) (Project, DeviceTemplate) {
	t.Helper()
	p := Template()
	p.Nodes[0].Bindings = []PropertyBinding{{Tag: "AI101", Property: "x", Steps: []point.Step{{Expr: "x*2"}}}}
	p.Links[0].Bends = []XY{{X: 350, Y: 240}}
	p.Sources = []Source{{ID: "sensor", URL: "http://127.0.0.1:1", Method: "GET", IntervalMS: 1000, TimeoutMS: 1000, Enabled: true, Mappings: []point.Mapping{{Name: "AI101", Kind: point.Float}}}}
	tpl, err := CaptureDeviceTemplate(p, []string{"reactor", "exchanger"}, "recovery", "Recovery skid")
	if err != nil {
		t.Fatal(err)
	}
	return p, tpl
}
func emptyPlant() Project { return Project{Version: Version, Width: 2000, Height: 1000} }

func TestDeviceCaptureNormalizesCompleteSubgraphWithoutDriversOrAliases(t *testing.T) {
	p, tpl := capturedDevice(t)
	before := editJSON(t, p)
	if len(tpl.Project.Nodes) != 2 || len(tpl.Project.Links) != 1 || len(tpl.Project.Points) != 2 || len(tpl.Project.Sources) != 0 || tpl.Project.RetentionDays != 0 {
		t.Fatal("wrong capture boundary", tpl)
	}
	if tpl.Project.Nodes[0].X != 0 || tpl.Project.Nodes[1].X != 310 || tpl.Project.Nodes[1].Y != 30 || tpl.Project.Links[0].Bends[0] != (XY{260, 180}) {
		t.Fatal("geometry not normalized", tpl)
	}
	tpl.Project.Nodes[0].Bindings[0].Steps[0].Expr = "x+1"
	tpl.Project.Links[0].Bends[0].X = 0
	if !bytes.Equal(before, editJSON(t, p)) {
		t.Fatal("capture aliased input")
	}
	for _, ids := range [][]string{nil, {"reactor", "reactor"}, {"unknown"}} {
		if _, err := CaptureDeviceTemplate(p, ids, "a", "A"); err == nil {
			t.Fatal("invalid capture accepted", ids)
		}
	}
}

func TestDevicePlacementRemapsPointsLinksAndIndependentInstances(t *testing.T) {
	_, tpl := capturedDevice(t)
	before := editJSON(t, tpl.Project)
	p := emptyPlant()
	q, placed, err := PlaceDeviceTemplate(p, tpl, DevicePlacement{ID: "unit1", X: 300, Y: 200, PointPrefix: "U1."})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.Points) != 2 || len(q.Sources) != 0 || q.Nodes[0].X != 300 || q.Nodes[1].X != 610 || q.Nodes[1].Y != 230 || q.Nodes[0].Tag != "U1.DI11" || q.Nodes[0].Bindings[0].Tag != "U1.AI101" || q.Links[0].From.Node != "unit1.reactor" || q.Links[0].Tag != "U1.DI11" || q.Links[0].Bends[0] != (XY{560, 380}) {
		t.Fatal("incorrect device placement", q)
	}
	if len(placed.Nodes) != 2 || placed.Nodes[0] != "unit1.reactor" || placed.Points["AI101"] != "U1.AI101" {
		t.Fatal(placed)
	}
	r, second, err := PlaceDeviceTemplate(q, tpl, DevicePlacement{ID: "unit2", X: 1000, Y: 100})
	if err != nil || len(r.Nodes) != 4 || len(r.Points) != 4 || second.Points["DI11"] != "unit2.DI11" {
		t.Fatal(err, r, second)
	}
	r.Nodes[0].Bindings[0].Steps[0].Expr = "x+9"
	r.Links[0].Bends[0].X = 0
	if q.Nodes[0].Bindings[0].Steps[0].Expr != "x*2" || !bytes.Equal(before, editJSON(t, tpl.Project)) || len(p.Nodes) != 0 {
		t.Fatal("placement aliases input/template")
	}
}

func TestDeviceExplicitSharedPointsRetainHostDefinitionAndWriter(t *testing.T) {
	p, tpl := capturedDevice(t)
	p.Nodes, p.Links = nil, nil
	p.Points[0].Initial = false
	p.Points[0].StaleMS = 3000
	p.Points[0].Description = "host owns this point"
	q, _, err := PlaceDeviceTemplate(p, tpl, DevicePlacement{ID: "shared", Points: map[string]string{"DI11": "DI11", "AI101": "AI101"}})
	if err != nil || len(q.Points) != 2 || len(q.Sources) != 1 || q.Points[0].Initial != false || q.Points[0].StaleMS != 3000 || q.Points[0].Description != p.Points[0].Description {
		t.Fatal("host point ownership changed", err, q)
	}
	if q.Nodes[1].Tag != "DI11" || q.Nodes[0].Bindings[0].Tag != "AI101" {
		t.Fatal("explicit binding lost")
	}
}

func TestDevicePlacementRejectedEditsRemainAtomic(t *testing.T) {
	_, tpl := capturedDevice(t)
	p := emptyPlant()
	before := editJSON(t, p)
	for _, o := range []DevicePlacement{{}, {ID: "x", X: math.NaN()}, {ID: "x", X: -1}, {ID: "x", X: 1900}, {ID: "x", Points: map[string]string{"missing": "name"}}, {ID: "x", Points: map[string]string{"DI11": "same", "AI101": "same"}}, {ID: strings.Repeat("x", 256)}, {ID: "x", PointPrefix: strings.Repeat("界", 86)}} {
		if _, _, err := PlaceDeviceTemplate(p, tpl, o); err == nil {
			t.Fatal("invalid placement accepted", o)
		}
		if !bytes.Equal(before, editJSON(t, p)) {
			t.Fatal("failed placement mutated original")
		}
	}
	p, _, _ = PlaceDeviceTemplate(p, tpl, DevicePlacement{ID: "x"})
	if _, _, err := PlaceDeviceTemplate(p, tpl, DevicePlacement{ID: "x"}); err == nil {
		t.Fatal("repeat instance/collisions accepted")
	}
	p = emptyPlant()
	p.Points = []Point{{Definition: point.Definition{Name: "target", Kind: point.Float, Unit: "different", Category: "AI"}}}
	if _, _, err := PlaceDeviceTemplate(p, tpl, DevicePlacement{ID: "x", Points: map[string]string{"AI101": "target"}}); err == nil {
		t.Fatal("unit mismatch accepted")
	}
	tpl.Project.Sources = []Source{{}}
	if _, _, err := PlaceDeviceTemplate(emptyPlant(), tpl, DevicePlacement{ID: "x"}); err == nil {
		t.Fatal("driver recipe accepted")
	}
}

func TestDeviceLibraryPortableIOBudgetsAndExactIntegers(t *testing.T) {
	lib := BuiltinDeviceLibrary()
	if _, err := lib.JSON(); err != nil {
		t.Fatal(err)
	}
	lib.Devices[0].Project.Points = append(lib.Devices[0].Project.Points, Point{Definition: point.Definition{Name: "COUNT", Kind: point.Int, Category: "BI"}, Initial: int64(math.MaxInt64)})
	copy, err := CloneDeviceLibrary(lib)
	if err != nil || copy.Devices[0].Project.Points[3].Initial != int64(math.MaxInt64) {
		t.Fatal(err, "library clone lost exact scalar")
	}
	copy.Devices[0].Project.Nodes[0].Label = "modified"
	if lib.Devices[0].Project.Nodes[0].Label == "modified" {
		t.Fatal("library aliases metadata")
	}
	path := filepath.Join(t.TempDir(), "devices.silkhmilibrary")
	ctx := context.Background()
	if err = SaveDeviceLibrary(ctx, path, lib, false); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadDeviceLibrary(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	count, err := point.Convert(loaded.Devices[0].Project.Points[3].Initial, point.Int)
	if err != nil || count != int64(math.MaxInt64) {
		t.Fatal("persisted integer precision lost", count, err)
	}
	before, _ := os.ReadFile(path)
	if err = SaveDeviceLibrary(ctx, path, copy, false); err == nil {
		t.Fatal("overwrite without consent")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err = SaveDeviceLibrary(cancelled, path, copy, true); err == nil {
		t.Fatal("cancelled write accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected save replaced file")
	}
	if err = SaveDeviceLibrary(ctx, path, copy, true); err != nil {
		t.Fatal(err)
	}
	b, _ := lib.JSON()
	for _, bad := range [][]byte{append(b, []byte(` {}`)...), []byte(`{"version":1,"devices":[],"typo":1}`), bytes.Repeat([]byte(" "), MaxDeviceLibraryBytes+1)} {
		if _, err = ParseDeviceLibrary(bad); err == nil {
			t.Fatal("malformed/oversized library accepted")
		}
	}
	lib.Devices = append(lib.Devices, lib.Devices[0])
	if _, err = lib.JSON(); err == nil {
		t.Fatal("duplicate IDs accepted")
	}
	if _, err = LoadDeviceLibrary(ctx, filepath.Dir(path)); err == nil {
		t.Fatal("directory accepted")
	}
	if _, err = json.Marshal(copy); err != nil {
		t.Fatal(err)
	}
}

func TestDeviceBuiltinInstancesSaveLoadAndBudgetRejection(t *testing.T) {
	p := emptyPlant()
	lib := BuiltinDeviceLibrary()
	for i, tpl := range lib.Devices {
		var err error
		p, _, err = PlaceDeviceTemplate(p, tpl, DevicePlacement{ID: fmt.Sprintf("device%d", i), X: float64(i) * 650, Y: 100})
		if err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "plant.silkhmi")
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || len(loaded.Nodes) != 11 || len(loaded.Points) != 7 || len(loaded.Sources) != 0 {
		t.Fatal(err, loaded)
	}
	before := editJSON(t, p)
	huge := emptyPlant()
	for i := 0; i < 511; i++ {
		huge.Nodes = append(huge.Nodes, Node{ID: fmt.Sprint(i), Kind: "text", Width: 16, Height: 16})
	}
	if _, _, err = PlaceDeviceTemplate(huge, lib.Devices[0], DevicePlacement{ID: "overflow"}); err == nil || len(huge.Nodes) != 511 {
		t.Fatal("device budget not atomic")
	}
	if !bytes.Equal(before, editJSON(t, p)) {
		t.Fatal("budget rejection changed previous document")
	}
	lib.Devices = nil
	for i := 0; i < MaxDeviceTemplates+1; i++ {
		lib.Devices = append(lib.Devices, DeviceTemplate{})
	}
	if _, err = lib.JSON(); err == nil {
		t.Fatal("library device budget ignored")
	}
}
