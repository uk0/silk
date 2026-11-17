package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/hmiapp"
	"github.com/uk0/silk/point"
)

func TestStatusProjectRoundtripAndGeneratedWorkbench(t *testing.T) {
	p := statusProject("http://127.0.0.1:1")
	b, err := p.JSON()
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := hmi.Parse(b)
	if err != nil || loaded.Nodes[1].Steps[0].Expr != "(x >> 4) & 0xf" {
		t.Fatal("status expression lost", err)
	}
	rt, err := hmi.NewRuntime(context.Background(), loaded, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop() // inert construction must not contact the unavailable endpoint.
	if _, err := hmiapp.Generate(loaded); err != nil {
		t.Fatal(err)
	}
	pipe, err := point.NewPipeline(loaded.Sources[0].Mappings[0].Steps)
	if err != nil {
		t.Fatal(err)
	}
	if v, err := pipe.Apply(int64(9007199254740993)); err != nil || v != int64(9007199254740995) {
		t.Fatal(v, err)
	}
}

func TestStatusDemoResponseBodiesRemainExactTokens(t *testing.T) {
	for _, mode := range []string{"normal", "overflow", "recover"} {
		r := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"device":"R001","mode":"`+mode+`"}`))
		w := httptest.NewRecorder()
		statusReply(w, r)
		b, _ := io.ReadAll(w.Result().Body)
		if w.Code != http.StatusOK || !bytes.Contains(b, []byte(`"counter":`)) || bytes.Contains(b, []byte("e+")) {
			t.Fatal("demo counter was float encoded", mode, w.Code, string(b))
		}
	}
}
