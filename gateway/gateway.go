// Package gateway bridges two field devices: it polls a source device into a
// shared core.TagDB and forwards every tag change on to a sink device, turning
// silk into a protocol gateway (数据转发 / FameView 数据转发). Source and sink
// points are matched by tag name, so a value read from one device's address is
// written to the other device's address for the same tag.
//
// This is deliberately NOT driver.Poller's read-write echo case. The Poller
// suppresses a poll's own tag update so a read never bounces back as a write to
// the SAME device; the gateway instead forwards every source change on to a
// DIFFERENT device. Forwarding is one-directional: source -> sink only.
package gateway

import (
	"sync"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/driver"
)

// Gateway forwards tags read from a source device to a sink device. It composes
// a driver.Poller (source device -> shared TagDB) with a set of tag
// subscriptions that write each change out to the sink device.
type Gateway struct {
	src    *driver.Poller             // polls the source device into tags
	sink   driver.Driver              // forwarding target
	tags   *core.TagDB                // shared tag database
	sinkBy map[string]driver.TagPoint // tag name -> sink point (the match table)

	wireOnce sync.Once
	cancels  []core.CancelFunc // forward subscriptions, released on Stop
}

// NewGateway builds a gateway. The source Poller reads srcPoints from source
// into tags every period; sinkPoints are indexed by tag name so each source tag
// is forwarded to the sink point that shares its name (same tag name = a match;
// the device addresses differ). period is the source read interval.
func NewGateway(source, sink driver.Driver, srcPoints, sinkPoints []driver.TagPoint, tags *core.TagDB, period time.Duration) *Gateway {
	sinkBy := make(map[string]driver.TagPoint, len(sinkPoints))
	for _, p := range sinkPoints {
		sinkBy[p.Tag] = p
	}
	return &Gateway{
		src:    driver.NewPoller(source, srcPoints, tags, period),
		sink:   sink,
		tags:   tags,
		sinkBy: sinkBy,
	}
}

// wireForwards subscribes each sink point's tag to a sink write, exactly once.
// The subscription's priming callback carries the tag's current value; a
// not-yet-polled tag primes nil, which is skipped so merely wiring the gateway
// does not write a bogus value to the sink.
func (g *Gateway) wireForwards() {
	g.wireOnce.Do(func() {
		for tag, sp := range g.sinkBy {
			sp := sp
			t := g.tags.GetOrCreate(tag, core.Meta{})
			c := t.Subscribe(func(v core.Value) {
				if v.Raw == nil {
					return // empty priming sample: nothing read from source yet
				}
				if err := g.sink.WritePoint(sp, v.Raw); err != nil {
					core.Warn("gateway forward ", sp.Tag, ": ", err)
				}
			})
			g.cancels = append(g.cancels, c)
		}
	})
}

// Start connects the sink, wires the forwards and starts the source Poller. It
// returns any sink-connect or source-start error; on a source failure the sink
// is closed again so Start never leaves the gateway half-open.
func (g *Gateway) Start() error {
	if err := g.sink.Connect(); err != nil {
		return err
	}
	g.wireForwards()
	if err := g.src.Start(); err != nil {
		g.sink.Close()
		return err
	}
	return nil
}

// ForwardOnce reads every source point once and forwards the changes to the
// sink synchronously (subscribers fire on the calling goroutine). It wires the
// forwards on first use, giving tests a deterministic source -> sink path with
// no poll goroutine and no sleeps.
func (g *Gateway) ForwardOnce() {
	g.wireForwards()
	g.src.PollOnce()
}

// Stop halts the source Poller (which closes the source device), cancels the
// forward subscriptions and closes the sink device. Safe to call once.
func (g *Gateway) Stop() {
	g.src.Stop()
	for _, c := range g.cancels {
		c()
	}
	g.cancels = nil
	g.sink.Close()
}
