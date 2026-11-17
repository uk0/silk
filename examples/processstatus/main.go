// A local, opt-in HTTP status-word and exact BI counter workbench.
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/hmiapp"
	"github.com/uk0/silk/point"
)

func statusProject(url string) hmi.Project {
	return hmi.Project{Version: hmi.Version, Title: "Exact BI counters and status words", Width: 1000, Height: 620,
		Points: []hmi.Point{
			{Definition: point.Definition{Name: "BI_COUNTER", Kind: point.Int, Category: "BI", Unit: "pcs"}, Initial: int64(0)},
			{Definition: point.Definition{Name: "BI_WORD", Kind: point.Int, Category: "BI"}, Initial: int64(0)},
			{Definition: point.Definition{Name: "DI_RUN", Kind: point.Bool, Category: "DI"}, Initial: false},
		}, Sources: []hmi.Source{{ID: "local_status", URL: url, Method: "POST", Body: `{"device":"R001","mode":"normal"}`, IntervalMS: 1000, TimeoutMS: 5000, Enabled: true, RepeatWrites: true, Mappings: []point.Mapping{
			{Name: "BI_COUNTER", Pointer: "/data/counter", Kind: point.Int, Steps: []point.Step{{Expr: "x+2"}}},
			{Name: "BI_WORD", Pointer: "/data/word", Kind: point.Int},
			{Name: "DI_RUN", Pointer: "/data/word", Kind: point.Bool, Steps: []point.Step{{Expr: "(x & 1) != 0"}}},
		}}}, Nodes: []hmi.Node{
			{ID: "counter", Kind: "value", Label: "Exact BI counter", X: 40, Y: 40, Width: 610, Height: 130, Tag: "BI_COUNTER", Unit: "pcs", Steps: []point.Step{{Expr: "x-2"}}, Colors: []hmi.ColorRule{{When: "x > 9007199254740992", Color: "#ef5350"}, {When: "true", Color: "#26a269"}}},
			{ID: "word", Kind: "value", Label: "Status upper nibble: (x >> 4) & 0xf", X: 40, Y: 210, Width: 610, Height: 130, Tag: "BI_WORD", Steps: []point.Step{{Expr: "(x >> 4) & 0xf"}}, Colors: []hmi.ColorRule{{When: "x == 10", Color: "#2196f3"}, {When: "true", Color: "#26a269"}}},
			{ID: "pump", Kind: "pump", Label: "DI_RUN: status bit 0", X: 710, Y: 70, Width: 220, Height: 220, Tag: "DI_RUN"},
			{ID: "help", Kind: "text", Label: "Run acquires from a local stand-in only. Stop before editing HTTP sources.\nRequest modes: normal / overflow / recover. No PLC commands.", X: 40, Y: 410, Width: 890, Height: 130},
		}}
}

func statusReply(w http.ResponseWriter, r *http.Request) {
	var body struct{ Device, Mode string }
	if r.Method != "POST" || json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&body) != nil || body.Device != "R001" {
		http.Error(w, "expected local demo POST body", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch body.Mode {
	case "normal":
		fmt.Fprint(w, `{"data":{"counter":9007199254740993,"word":165}}`)
	case "overflow":
		fmt.Fprint(w, `{"data":{"counter":9223372036854775807,"word":8}}`)
	case "recover":
		fmt.Fprint(w, `{"data":{"counter":9007199254740992,"word":1}}`)
	default:
		http.Error(w, "mode must be normal, overflow or recover", http.StatusBadRequest)
	}
}

func main() {
	// The listener is a loopback stand-in. Point storage/acquisition starts only
	// after the workbench's explicit Run action, never while editing the document.
	server := httptest.NewServer(http.HandlerFunc(statusReply))
	defer server.Close()
	doc, err := statusProject(server.URL).JSON()
	if err != nil {
		log.Fatal(err)
	}
	if err := hmiapp.Run(hmiapp.Config{ProjectJSON: string(doc)}); err != nil {
		log.Fatal(err)
	}
}
