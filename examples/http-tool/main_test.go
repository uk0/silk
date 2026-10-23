package main

import (
	"context"
	"github.com/uk0/silk/httpx"
	"testing"
)

func TestToolAssemblyBoundsAndDemoService(t *testing.T) {
	url, stop, err := demoServer()
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	f := newHTTPTool(url)
	defer f.send.Close()
	for _, size := range [][2]float64{{0, 0}, {90, 80}, {440, 360}, {780, 520}, {1200, 800}} {
		f.SetSize(size[0], size[1])
		for _, child := range f.Children() {
			x, y, w, h := child.Bounds()
			if x < 0 || y < 0 || w < 0 || h < 0 || x+w > size[0] || y+h > size[1] {
				t.Fatalf("child outside %v: %v %v %v %v", size, x, y, w, h)
			}
		}
	}
	response, err := httpx.Default().Get(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	if count, err := response.Int("/data/count"); err != nil || count != 9007199254740993 {
		t.Fatal(count, err)
	}
}
