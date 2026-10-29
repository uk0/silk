package scene3d

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
)

func TestBuiltinDeviceTemplatesSharedInertAndBindable(t *testing.T) {
	defs, err := BuiltinDeviceTemplates()
	mustEdit(t, err)
	if len(defs) != 3 {
		t.Fatal("missing builtins")
	}
	for _, def := range defs {
		t.Run(def.ID, func(t *testing.T) {
			stats := def.Template.Stats()
			if stats.Nodes != def.Template.NodeCount() || stats.Meshes == 0 || stats.Vertices == 0 || stats.Triangles == 0 || stats.Bindings == 0 || stats.EstimatedBytes <= stats.Vertices*24 || len(def.Template.Warnings()) == 0 {
				t.Fatal(stats)
			}
			d := &Document{Version: DocumentVersion}
			e, err := NewEditSession(context.Background(), d)
			mustEdit(t, err)
			defer e.Close()
			_, err = e.InstantiateTemplates(context.Background(), def.Template, []InstanceOptions{{ID: "first", TagPrefix: "one."}, {ID: "second", TagPrefix: "two."}})
			mustEdit(t, err)
			if len(e.MeshIDs()) != stats.Meshes || len(e.Bindings()) != stats.Bindings*2 {
				t.Fatal("resource/binding accounting")
			}
			live, _ := e.Scene()
			program, err := NewBindingSet(live, e.Bindings())
			mustEdit(t, err)
			authored, _ := e.Document()
			for _, tag := range def.Template.Tags() {
				var value any = 50.0
				if tag.Boolean {
					value = true
				}
				_, err = program.Apply("one."+tag.Name, value)
				mustEdit(t, err)
			}
			after, _ := e.Document()
			if !reflect.DeepEqual(authored, after) {
				t.Fatal("live program changed authored definition")
			}
			if _, err = live.Snapshot(); err != nil {
				t.Fatal(err)
			}
		})
	}
	first := defs[0].Template
	defs[0].ID = "changed"
	again, err := BuiltinDeviceTemplates()
	mustEdit(t, err)
	if again[0].ID != "pump" || again[0].Template != first {
		t.Fatal("builtin metadata alias or recompiled meshes")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := BuiltinDeviceTemplates()
			if err != nil || items[0].Template != first {
				t.Error("concurrent definitions")
			}
			_ = items[0].Template.Stats()
			_ = items[0].Template.Tags()
		}()
	}
	wg.Wait()
}

func BenchmarkEditTemplateBatch1000(b *testing.B) {
	defs, err := BuiltinDeviceTemplates()
	mustEdit(b, err)
	options := make([]InstanceOptions, 1000)
	for i := range options {
		options[i] = InstanceOptions{ID: fmt.Sprint("pump-", i), TagPrefix: fmt.Sprint("pump.", i, ".")}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		e, err := NewEditSession(context.Background(), &Document{Version: DocumentVersion})
		if err != nil {
			b.Fatal(err)
		}
		_, err = e.InstantiateTemplates(context.Background(), defs[0].Template, options)
		if err != nil {
			b.Fatal(err)
		}
		e.Close()
	}
}
