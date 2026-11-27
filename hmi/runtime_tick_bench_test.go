package hmi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/uk0/silk/point"
)

func BenchmarkRuntimeTickPointFreshness(b *testing.B) {
	for _, cases := range []struct {
		name  string
		stale int
	}{{"none", 0}, {"sparse", 1}, {"all", 512}} {
		b.Run(cases.name, func(b *testing.B) {
			p := Project{Version: Version, Title: "Point clock QA", Width: 640, Height: 480}
			for i := range 512 {
				d := Point{Definition: point.Definition{Name: fmt.Sprintf("AI%03d", i), Kind: point.Float}}
				if i < cases.stale {
					d.StaleMS = 1000
				}
				p.Points = append(p.Points, d)
			}
			rt, err := NewRuntime(context.Background(), p, nil, nil)
			if err != nil {
				b.Fatal(err)
			}
			defer rt.Stop()
			now := time.Now()
			b.ReportAllocs()
			for b.Loop() {
				rt.Tick(now)
			}
		})
	}
}
