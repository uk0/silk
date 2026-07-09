package driver

import (
	"sync"
	"time"

	"github.com/uk0/silk/core"
)

// AccessMode is a tag point's read/write capability.
type AccessMode int

const (
	ReadOnly AccessMode = iota
	ReadWrite
)

func (a AccessMode) String() string {
	if a == ReadWrite {
		return "RW"
	}
	return "RO"
}

// TagPoint maps one device address to one silk tag with its data type, byte
// order and access mode. It is the unit of device configuration.
type TagPoint struct {
	Tag     string    // silk tag name this point drives
	Address string    // protocol address (Modbus "40001"; S7 "DB1.DBD0")
	Type    DataType  // value type
	Order   ByteOrder // register/byte order for multi-byte types
	Access  AccessMode
}

// Writable reports whether tag edits are pushed back to the device.
func (p TagPoint) Writable() bool { return p.Access == ReadWrite }

// Driver is a live connection to one field device. Implementations (Modbus TCP,
// S7) handle addressing and the wire protocol; the codec handles value bytes.
type Driver interface {
	Connect() error
	Close() error
	ReadPoint(p TagPoint) (interface{}, error)
	WritePoint(p TagPoint, v interface{}) error
}

// Poller drives a core.TagDB from a device: it periodically reads every point
// into its tag, and for read-write points pushes user/script tag edits back to
// the device. Poll-originated tag updates are suppressed from the write path so
// a read never echoes back as a write.
type Poller struct {
	drv    Driver
	points []TagPoint
	tags   *core.TagDB
	period time.Duration

	mu       sync.Mutex
	suppress map[string]bool // tags currently being set by the poll loop
	cancels  []core.CancelFunc
	stop     chan struct{}
	done     chan struct{}
}

// NewPoller builds a poller. period is the read interval.
func NewPoller(drv Driver, points []TagPoint, tags *core.TagDB, period time.Duration) *Poller {
	return &Poller{drv: drv, points: points, tags: tags, period: period, suppress: map[string]bool{}}
}

// Start connects the device, wires read-write points to device writes, and
// launches the poll loop. It errors if the device connection fails.
func (p *Poller) Start() error {
	if err := p.drv.Connect(); err != nil {
		return err
	}
	p.wireWrites()
	p.stop = make(chan struct{})
	p.done = make(chan struct{})
	go p.loop()
	return nil
}

// wireWrites subscribes each read-write point's tag to a device write, skipping
// poll-originated updates (the echo guard). Split from Start so the write path
// can be exercised without the poll goroutine.
func (p *Poller) wireWrites() {
	for _, pt := range p.points {
		if !pt.Writable() {
			continue
		}
		pt := pt
		tag := p.tags.GetOrCreate(pt.Tag, core.Meta{})
		// Subscribe fires a priming callback with the current value; guard it so
		// merely wiring a point does not write to the device.
		p.mu.Lock()
		p.suppress[pt.Tag] = true
		p.mu.Unlock()
		c := tag.Subscribe(func(v core.Value) {
			p.mu.Lock()
			echo := p.suppress[pt.Tag]
			p.mu.Unlock()
			if echo {
				return // poll-originated or priming update, not a user edit
			}
			if err := p.drv.WritePoint(pt, v.Raw); err != nil {
				core.Warn("driver write ", pt.Tag, ": ", err)
			}
		})
		p.mu.Lock()
		delete(p.suppress, pt.Tag)
		p.mu.Unlock()
		p.cancels = append(p.cancels, c)
	}
}

// PollOnce reads every point once into the tag DB. Exposed for tests and for a
// synchronous initial refresh; the loop calls it each tick.
func (p *Poller) PollOnce() {
	for _, pt := range p.points {
		v, err := p.drv.ReadPoint(pt)
		if err != nil {
			core.Warn("driver read ", pt.Tag, ": ", err)
			continue
		}
		tag := p.tags.GetOrCreate(pt.Tag, core.Meta{})
		p.mu.Lock()
		p.suppress[pt.Tag] = true
		p.mu.Unlock()
		tag.SetValue(tagValue(v, pt.Type)) // subscribers fire synchronously; suppress skips the write-back
		p.mu.Lock()
		delete(p.suppress, pt.Tag)
		p.mu.Unlock()
	}
}

func (p *Poller) loop() {
	defer close(p.done)
	t := time.NewTicker(p.period)
	defer t.Stop()
	p.PollOnce()
	for {
		select {
		case <-p.stop:
			return
		case <-t.C:
			p.PollOnce()
		}
	}
}

// Stop halts polling, unsubscribes write handlers and closes the device. It is
// safe to call once.
func (p *Poller) Stop() {
	if p.stop != nil {
		close(p.stop)
		<-p.done
		p.stop = nil
	}
	for _, c := range p.cancels {
		c()
	}
	p.cancels = nil
	p.drv.Close()
}
