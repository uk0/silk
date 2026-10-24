package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/uk0/silk/httpx"
	"github.com/uk0/silk/jsonx"
)

func TestJSONViewLoadSelectionKeyboardAndFailureSafety(t *testing.T) {
	v := NewJSONView()
	defer v.Close()
	v.SetSize(700, 360)
	v.SetJSONText(`{"a/b":{"~":[1,"中文😀",true]},"z":null}`)
	var selected []string
	v.SigSelected(func(p string) { selected = append(selected, p) })
	if err := v.SelectPointer("/a~1b/~0/1"); err != nil {
		t.Fatal(err)
	}
	n, ok := v.SelectedNode()
	if !ok || n.Preview(30) != `"中文😀"` {
		t.Fatal(n, ok)
	}
	if got, err := v.SelectedJSON(); err != nil || got != `"中文😀"` {
		t.Fatal(got, err)
	}
	v.OnKeyDown(KeyDown, false)
	if n, _ := v.SelectedNode(); n.Pointer() != "/a~1b/~0/2" {
		t.Fatal(n.Pointer())
	}
	if len(selected) != 2 {
		t.Fatal("keyboard selection signal missing/duplicated", selected)
	}
	before := v.Document()
	rows := len(v.rows)
	sample := v.JSONText()
	errors := 0
	v.SigError(func(string) { errors++ })
	v.SetJSONText("{broken")
	if v.LastError() == nil || v.Document() != before || len(v.rows) != rows || v.JSONText() != sample || errors != 1 {
		t.Fatal("invalid load replaced usable document")
	}
	v.SetRootPointer("/a~1b")
	if err := v.SelectPointer("/z"); err == nil {
		t.Fatal("selection outside subtree accepted")
	}
	if err := v.SelectPointer("/a~1b/~0/0"); err != nil {
		t.Fatal(err)
	}
	v.SetRootPointer("")
	if err := v.LoadJSON(`{"runtime":"do-not-persist"}`); err != nil {
		t.Fatal(err)
	}
	if v.JSONText() != sample {
		t.Fatal("runtime payload became a design property")
	}
}
func TestJSONViewLargeArrayIsGroupedAndLazy(t *testing.T) {
	values := make([]int, 10000)
	for i := range values {
		values[i] = i
	}
	data, _ := json.Marshal(values)
	v := NewJSONView()
	defer v.Close()
	v.SetSize(600, 300)
	v.SetPageSize(32)
	if err := v.LoadJSON(string(data)); err != nil {
		t.Fatal(err)
	}
	if len(v.rows) > 33 || len(v.rmap) > 33 {
		t.Fatal("eagerly materialized whole array", len(v.rows), len(v.rmap))
	}
	if err := v.SelectPointer("/9999"); err != nil {
		t.Fatal(err)
	}
	n, ok := v.SelectedNode()
	if !ok {
		t.Fatal("missing selected node")
	}
	if value, err := n.Int(); err != nil || value != 9999 {
		t.Fatal(value, err)
	}
	if len(v.rows) > 100 || len(v.rmap) > 100 {
		t.Fatal("point lookup materialized unrelated ranges", len(v.rows), len(v.rmap))
	}
	for _, row := range v.rows {
		if v.jsonModel.RowCount(row.mi) > 32 {
			t.Fatal("fan-out exceeds page size")
		}
	}
	v.SetCurrentRow(1)
	if _, ok := v.SelectedNode(); ok {
		t.Fatal("synthetic group reported as a real JSON value")
	}
	if _, err := v.SelectedJSON(); err == nil {
		t.Fatal("group silently copied wrong subtree")
	}
}
func TestJSONViewModelBoundsAndEscapedKeys(t *testing.T) {
	v := NewJSONView()
	defer v.Close()
	v.SetJSONText(`{"a\nb": "line\nbreak", "large": 9007199254740993}`)
	m := v.jsonModel
	root := m.Index(0, 0, ModelIndex{})
	child := m.Index(0, 0, root)
	if got := m.Data(child, DisplayRole).(string); strings.Contains(got, "\n") || !strings.Contains(got, `\n`) {
		t.Fatal("key breaks row layout", got)
	}
	if m.Parent(child) != root || m.RowCount(child) != 0 || !m.Index(-1, 0, root).IsNil() || !m.Index(0, 3, root).IsNil() {
		t.Fatal("model index contract")
	}
	foreign := NewJSONView()
	defer foreign.Close()
	if m.RowCount(foreign.jsonModel.index(foreign.jsonModel.root, 0)) != 0 {
		t.Fatal("foreign model index accepted")
	}
	v.SetPageSize(-1)
	if v.PageSize() != 16 {
		t.Fatal(v.PageSize())
	}
	v.SetPageSize(1000000)
	if v.PageSize() != 256 {
		t.Fatal(v.PageSize())
	}
}
func TestJSONViewHTTPBackgroundParseUIQueue(t *testing.T) {
	resetUIQueue()
	defer resetUIQueue()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"name":"Silk 中文😀","count":9007199254740993}}`)
	}))
	defer server.Close()
	v := NewJSONView()
	defer v.Close()
	before := v.Document()
	done := false
	future, err := RunAsync(context.Background(), nil, func(ctx context.Context) (*jsonx.Document, error) {
		r, err := httpx.Default().Get(ctx, server.URL)
		if err != nil {
			return nil, err
		}
		return r.Document()
	}, func(doc *jsonx.Document, err error) {
		if err != nil {
			t.Error(err)
		} else if err = v.SetDocument(doc); err != nil {
			t.Error(err)
		}
		done = true
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := future.Await(context.Background()); err != nil {
		t.Fatal(err)
	}
	if done || v.Document() != before {
		t.Fatal("worker modified UI outside dispatch")
	}
	pumpHTTP(t, func() bool { return done })
	if err := v.SelectPointer("/data/count"); err != nil {
		t.Fatal(err)
	}
	n, _ := v.SelectedNode()
	if value, _ := n.Int(); value != 9007199254740993 {
		t.Fatal(value)
	}
}
func BenchmarkJSONViewLargeArrayReveal(b *testing.B) {
	data := []byte("[" + strings.Repeat("0,", 9999) + "1]")
	doc, _ := jsonx.Parse(data)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := NewJSONView()
		_ = v.SetDocument(doc)
		_ = v.SelectPointer("/9999")
		v.Close()
	}
}

func TestJSONViewColumnsFollowResizeWithoutLosingSelection(t *testing.T) {
	v := NewJSONView()
	defer v.Close()
	v.SetJSONText(`{"a":1}`)
	v.SetSize(400, 300)
	v.Layout()
	if err := v.SelectPointer("/a"); err != nil {
		t.Fatal(err)
	}
	before := v.header.TotalSectionSize()
	rows := len(v.rows)
	v.SetSize(800, 300)
	v.Layout()
	if v.header.TotalSectionSize() <= before+300 {
		t.Fatal("columns retain their initial narrow widths")
	}
	if n, ok := v.SelectedNode(); !ok || n.Pointer() != "/a" || len(v.rows) != rows {
		t.Fatal("resize changed selection/expansion")
	}
}
