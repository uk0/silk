package hmi

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

const indexedFixture = `{
 "version":1,"title":"🏭 no ID here","width":600,"height":300,"points":[],
 "links":[{"tag":"","from":{"node":"设备","port":"east"},"to":{"node":"b","port":"west"},"id":"设备"}],
 "nodes":[
   {"label":"🔧 \"id\": \"decoy\"","id":"\u8bbe备","kind":"value","x":20,"y":20,"width":40,"height":40},
   {"id":"b","label":"line test","kind":"text","x":100,"y":20,"width":40,"height":40}
 ]
}`

func indexedPosition(s string, offset int) (line, column int) {
	return strings.Count(s[:offset], "\n"), offset - (strings.LastIndexByte(s[:offset], '\n') + 1)
}

func TestIndexedSourceUnicodeCRLFScopesAndKind(t *testing.T) {
	for _, s := range []string{indexedFixture, strings.ReplaceAll(indexedFixture, "\n", "\r\n"), strings.ReplaceAll(indexedFixture, "\n", "")} {
		p, index, err := ParseIndexedSource(s)
		if err != nil || len(p.Nodes) != 2 || len(p.Links) != 1 {
			t.Fatal(err, p)
		}
		for _, check := range []struct {
			idMarker, inside string
			target           SourceTarget
		}{
			{`"id":"\u8bbe备"`, "decoy", SourceTarget{SourceNode, "设备"}},
			{`"id":"设备"`, `"port":"east"`, SourceTarget{SourceLink, "设备"}},
			{`"id":"b"`, "line test", SourceTarget{SourceNode, "b"}},
		} {
			line, col := indexedPosition(s, strings.Index(s, check.inside))
			if got, ok := index.AtPosition(line, col); !ok || got != check.target {
				t.Fatal(got, ok, check)
			}
			offset := strings.Index(s, check.idMarker) + len(`"id":"`)
			line, col = indexedPosition(s, offset)
			runes := utf8.RuneCountInString(s[offset-col : offset])
			if y, x, ok := index.IDPosition(check.target); !ok || y != line || x != runes {
				t.Fatal(y, x, ok, line, runes, check)
			}
		}
		for _, off := range []int{0, strings.Index(s, "no ID here"), strings.Index(s, `,"nodes"`), len(s)} {
			if off < 0 {
				continue
			}
			line, col := indexedPosition(s, off)
			if got, ok := index.AtPosition(line, col); ok {
				t.Fatal("outside object selected", got, off)
			}
		}
		if _, ok := index.AtPosition(9999, 0); ok {
			t.Fatal("invalid line")
		}
		if _, ok := index.AtPosition(0, 999999); ok {
			t.Fatal("invalid column")
		}
		if _, _, ok := index.IDPosition(SourceTarget{SourceNode, "decoy"}); ok {
			t.Fatal("escaped label treated as ID")
		}
	}
}

func TestIndexedSourceDuplicateArrayAndIDUseEffectiveToken(t *testing.T) {
	s := `{"version":1,"title":"shadowed","width":200,"height":200,
"nodes":[{"id":"superseded","label":"old","kind":"value","x":20,"y":20,"width":40,"height":40}],
"NoDeS":[{"ID":"wrong","i\u0064":"effective","id":null}],"links":[]}`
	p, index, err := ParseIndexedSource(s)
	if err != nil || len(p.Nodes) != 1 || p.Nodes[0].ID != "effective" {
		t.Fatal(p, err)
	}
	if _, ok := index.AtPosition(1, 25); ok {
		t.Fatal("superseded array still indexed")
	}
	want := SourceTarget{SourceNode, "effective"}
	if line, col, ok := index.IDPosition(want); !ok || line != 2 || !strings.HasPrefix(strings.Split(s, "\n")[line][col:], "effective") {
		t.Fatal(line, col, ok)
	}
	// A duplicate array can inherit a previous element's missing fields. There
	// is no authored effective ID token in the final array; never guess.
	s = strings.Replace(s, `{"ID":"wrong","i\u0064":"effective","id":null}`, `{"label":"new"}`, 1)
	p, index, err = ParseIndexedSource(s)
	if err != nil || p.Nodes[0].ID != "superseded" {
		t.Fatal(p, err)
	}
	if _, _, ok := index.IDPosition(SourceTarget{SourceNode, "superseded"}); ok {
		t.Fatal("guessed inherited ID position")
	}
}

func TestIndexedSourceRetainsParseAcceptance(t *testing.T) {
	for _, s := range []string{"", "null", indexedFixture + "[]", indexedFixture + "}", strings.Repeat(" ", MaxDocumentBytes+1), strings.Replace(indexedFixture, `"id":"b"`, `"xx":"b"`, 1), strings.Replace(indexedFixture, `"width":600`, `"width":0`, 1)} {
		if _, i, e := ParseIndexedSource(s); e == nil || i != nil {
			t.Fatal("accepted invalid source", len(s), e)
		}
	}
	s := `{"version":1,"width":100,"height":100,"nodes":null,"links":null}`
	if _, i, e := ParseIndexedSource(s); e != nil || i == nil {
		t.Fatal("empty source lost", e)
	}
	var index *SourceIndex
	if _, ok := index.AtPosition(0, 0); ok {
		t.Fatal("nil index lookup")
	}
	if _, _, ok := index.IDPosition(SourceTarget{}); ok {
		t.Fatal("nil index ID")
	}
}

func TestIndexedSourceAlarmNamespaceAndExactPosition(t *testing.T) {
	s := strings.Replace(indexedFixture, `"points":[]`, `"points":[{"name":"设备","kind":"bool"}]`, 1)
	s = strings.Replace(s, `"nodes":[`, `"alarms":[{"id":"retired","point":"设备","condition":"is","priority":"warning","threshold":true}],
 "AlArMs":[{"point":"设备", "i\u0064":"设备","condition":"is","priority":"critical","threshold":false}],
 "nodes":[`, 1)
	s = strings.ReplaceAll(s, "\n", "\r\n")
	p, index, err := ParseIndexedSource(s)
	if err != nil || len(p.Alarms) != 1 || p.Alarms[0].ID != "设备" {
		t.Fatalf("alarm fixture: %v %v", p.Alarms, err)
	}
	target := SourceTarget{Kind: SourceAlarm, ID: "设备"}
	marker := `"i\u0064":"`
	offset := strings.Index(s, marker) + len(marker)
	line, byteColumn := indexedPosition(s, offset)
	runes := utf8.RuneCountInString(s[offset-byteColumn : offset])
	if y, x, ok := index.IDPosition(target); !ok || y != line || x != runes {
		t.Fatalf("alarm coordinate: %d:%d %v want %d:%d", y, x, ok, line, runes)
	}
	if got, ok := index.AtPosition(line, byteColumn); !ok || got != target {
		t.Fatal("alarm position mistyped", got, ok)
	}
	for _, kind := range []SourceEntity{SourceNode, SourceLink, SourcePoint} {
		if y, _, ok := index.IDPosition(SourceTarget{Kind: kind, ID: "设备"}); !ok || y == line {
			t.Fatal("alarm alias overlapped another namespace", kind, y, ok)
		}
	}
	if _, _, ok := index.IDPosition(SourceTarget{Kind: SourceAlarm, ID: "retired"}); ok {
		t.Fatal("indexed a superseded alarm array")
	}
}

func TestIndexedSourcePointDeclarationInItsOwnNamespace(t *testing.T) {
	s := strings.Replace(indexedFixture, `"points":[]`,
		`"points":[{"name":"shadowed","kind":"int"}],
 "POINTS":[{"description":"🏭 name decoy","N\u0041ME":"设备","name":null}]`, 1)
	s = strings.ReplaceAll(s, "\n", "\r\n")
	p, idx, err := ParseIndexedSource(s)
	if err != nil || len(p.Points) != 1 || p.Points[0].Name != "设备" {
		t.Fatal(p.Points, err)
	}
	name := SourceTarget{SourcePoint, "设备"}
	marker := `"N\u0041ME":"`
	offset := strings.Index(s, marker) + len(marker)
	line, col := indexedPosition(s, offset)
	wantCol := utf8.RuneCountInString(s[offset-col : offset])
	if y, x, ok := idx.IDPosition(name); !ok || y != line || x != wantCol {
		t.Fatal(y, x, ok, line, wantCol)
	}
	if got, ok := idx.AtPosition(line, col); !ok || got != name {
		t.Fatal(got, ok)
	}
	if nodeLine, _, ok := idx.IDPosition(SourceTarget{SourceNode, name.ID}); !ok || nodeLine == line {
		t.Fatal("crossed point and device namespaces", nodeLine)
	}
	inherited := strings.Replace(s, marker+`设备","name":null`, `"initial":0`, 1)
	_, idx, err = ParseIndexedSource(inherited)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok := idx.IDPosition(SourceTarget{SourcePoint, "shadowed"}); ok {
		t.Fatal("guessed inherited point name")
	}
}

func BenchmarkIndexedSource512Nodes(b *testing.B) {
	p := Project{Version: Version, Width: 100, Height: 100}
	for n := 0; n < 512; n++ {
		p.Nodes = append(p.Nodes, Node{ID: fmt.Sprint(n), Kind: "value", X: 20, Y: 20, Width: 40, Height: 40})
	}
	data, _ := json.Marshal(p)
	s := string(data)
	b.ReportAllocs()
	for b.Loop() {
		if _, _, e := ParseIndexedSource(s); e != nil {
			b.Fatal(e)
		}
	}
}
