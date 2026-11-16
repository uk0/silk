package ged

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
)

func TestCodeSnapshotMatchesAllFactoriesAndIndexedLines(t *testing.T) {
	keys := make([]string, 0, len(factoryMap))
	for factory := range factoryMap {
		keys = append(keys, factory)
	}
	sort.Strings(keys)
	for _, factory := range keys {
		t.Run(factory, func(t *testing.T) {
			s := NewGedScene()
			s.SetFormTitle("Plant 厂😀")
			s.SetSize(200, 150)
			f := addFakeTo(t, s, factory, "Probe")
			f.SetBounds(20, 25, 40, 12)
			if text, ok := f.Widget().(interface{ SetText(string) }); ok {
				text.SetText("Caption 厂😀")
			}
			if icon, ok := f.Widget().(interface{ SetIconName(string) }); ok {
				icon.SetIconName("save")
			}
			if tag, ok := f.Widget().(interface{ SetTagName(string) }); ok {
				tag.SetTagName("DI11")
			}
			captured := s.CaptureCodeSnapshot()
			assertCodeSnapshotValues(t, reflect.ValueOf(captured).Elem(), "snapshot")
			for _, field := range captured.fields {
				if field.widget != nil || field.fake != nil {
					t.Fatal("snapshot retains live widget")
				}
			}
			for _, opts := range []CodeGenOptions{{}, {PackageName: "panels", TypeName: "StableUI", SplitHandlers: true}} {
				indexed := s.GenerateCodeIndexed(opts)
				g := captured.Generate(opts)
				if g.Code != indexed.Code || fmt.Sprint(g.Err) != fmt.Sprint(indexed.Err) {
					t.Fatal("capture changed generated output/error")
				}
				if _, ok := indexed.Lines[f]; !ok {
					t.Fatal("lost UI constructor anchor")
				}
			}
		})
	}
}

// Prevent later additions from quietly retaining a scene, callback or a widget
// through an unrelated field. Nil UI collection slots are allowed, live object
// pointers/interfaces/functions/channels are not part of the captured contract.
func assertCodeSnapshotValues(t *testing.T, v reflect.Value, path string) {
	t.Helper()
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			t.Fatalf("%s retains a live reference of type %s", path, v.Type())
		}
	case reflect.Func, reflect.Chan, reflect.UnsafePointer:
		t.Fatalf("%s has worker-unsafe type %s", path, v.Type())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			assertCodeSnapshotValues(t, v.Field(i), path+"."+v.Type().Field(i).Name)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			assertCodeSnapshotValues(t, v.Index(i), fmt.Sprintf("%s[%d]", path, i))
		}
	case reflect.Map:
		for _, key := range v.MapKeys() {
			assertCodeSnapshotValues(t, key, path+".key")
			assertCodeSnapshotValues(t, v.MapIndex(key), path+".value")
		}
	}
}

func TestCodeSnapshotDefersHandlerParsingAndRetainsOriginalDiagnosticNames(t *testing.T) {
	s := NewGedScene()
	b := addFakeTo(t, s, "gui.Button", "original name 厂😀")
	b.SetCode("func invalid(")
	want := s.checkHandlerSource()
	captured := s.CaptureCodeSnapshot()
	if captured.errText != "" || captured.fields[0].code != b.GetCode() {
		t.Fatal("capture parsed or lost handler source")
	}
	done := make(chan CapturedCode, 1)
	go func() { done <- captured.Generate(CodeGenOptions{}) }()
	r := <-done
	if r.Err == nil || want == nil || r.Err.Error() != want.Error() {
		t.Fatal("worker changed handler diagnostic", r.Err, want)
	}
}

func TestCodeSnapshotConcurrentRenderDoesNotReadLiveState(t *testing.T) {
	s := NewGedScene()
	s.SetFormTitle("Captured")
	s.SetSize(200, 150)
	grid := addFakeTo(t, s, "gui.GridLayout", "Grid")
	grid.layoutManaged = true
	child := addFakeTo(t, s, "gui.Button", "Cell")
	child.SetParent(grid)
	child.Widget().(*gui.Button).SetText("Original")
	child.SetEventHandler("OnClick", "onCell")
	child.SetCode(`func onCell(){ println("captured body") }`)
	s.SetTagDict([]TagDecl{{Name: "DI11", Meta: core.Meta{Desc: "Original"}}})
	captured := s.CaptureCodeSnapshot()
	opts := []CodeGenOptions{{}, {PackageName: "panels", TypeName: "StableUI", SplitHandlers: true}}
	want := []string{captured.Generate(opts[0]).Code, captured.Generate(opts[1]).Code}
	var wg sync.WaitGroup
	start := make(chan struct{})
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for i := 0; i < 30; i++ {
				for j, opt := range opts {
					if g := captured.Generate(opt); g.Err != nil || g.Code != want[j] {
						t.Error("worker observed changing UI state", g.Err)
						return
					}
				}
			}
		}()
	}
	close(start)
	// The only live UI writes are on the test/UI owner goroutine. A race here
	// means capture leaked a widget, binding map, tag declaration or layout.
	for i := 0; i < 60; i++ {
		s.SetFormTitle(fmt.Sprint("Later", i))
		child.Widget().(*gui.Button).SetText(fmt.Sprint(i))
		child.EventHandlers()["OnClick"] = fmt.Sprint("later", i)
		child.SetCode("func later(){}")
		child.SetBounds(float64(i), 2, 3, 4)
		grid.layoutManaged = !grid.layoutManaged
		s.TagDict()[0].Meta.Desc = fmt.Sprint(i)
		s.SetTagDict([]TagDecl{{Name: fmt.Sprint("AI", i)}})
	}
	wg.Wait()
	if captured.fields[1].eventHandlers["OnClick"] != "onCell" || captured.decls[0].Name != "DI11" {
		t.Fatal("capture shares mutable input containers")
	}
}

func TestCodeSnapshotWorkerSplitWritesCapturedStateAndPreservesUser(t *testing.T) {
	s := NewGedScene()
	s.SetFormTitle("Captured")
	b := addFakeTo(t, s, "gui.Button", "Action")
	b.Widget().(*gui.Button).SetText("Captured caption")
	b.SetEventHandler("OnClick", "onAction")
	b.SetCode(`func onAction(){println("seed")}`)
	captured := s.CaptureCodeSnapshot()
	path := filepath.Join(t.TempDir(), "view.go")
	opts := CodeGenOptions{PackageName: "panels", TypeName: "StableUI"}
	type result struct {
		pair SplitResult
		err  error
	}
	run := func(regenerate bool) result {
		ch := make(chan result, 1)
		go func() {
			var r result
			if regenerate {
				r.pair, r.err = captured.RegenerateCodeFile(path, opts)
			} else {
				r.pair, r.err = captured.GenerateCodeFile(path, opts)
			}
			ch <- r
		}()
		return <-ch
	}
	r := run(false)
	if r.err != nil {
		t.Fatal(r.err)
	}
	user, err := os.ReadFile(r.pair.UserFile)
	if err != nil {
		t.Fatal(err)
	}
	owned := strings.ReplaceAll(string(user), `"seed"`, `"developer"`) + "\n// user spacing and notes\n"
	if err := os.WriteFile(r.pair.UserFile, []byte(owned), 0600); err != nil {
		t.Fatal(err)
	}
	s.SetFormTitle("Later UI")
	b.Widget().(*gui.Button).SetText("Later caption")
	if r = run(true); r.err != nil || r.pair.UserCreated {
		t.Fatal(r)
	}
	machine, err := os.ReadFile(r.pair.MachineFile)
	if err != nil || !strings.Contains(string(machine), "Captured caption") || strings.Contains(string(machine), "Later caption") {
		t.Fatal("worker read later UI", err)
	}
	after, err := os.ReadFile(r.pair.UserFile)
	if err != nil || string(after) != owned {
		t.Fatal("worker rewrote user content", err)
	}
	if err := os.Remove(r.pair.UserFile); err != nil {
		t.Fatal(err)
	}
	if r = run(true); r.err == nil {
		t.Fatal("automatic generation recreated missing user file")
	}
	if _, err := os.Stat(r.pair.UserFile); !os.IsNotExist(err) {
		t.Fatal("missing user file was synthesized", err)
	}
}
