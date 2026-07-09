package driver

import (
	"sync"
	"testing"
	"time"

	"github.com/uk0/silk/core"
)

// fakeDriver is an in-memory Driver for testing the Poller without a device.
type fakeDriver struct {
	mu     sync.Mutex
	vals   map[string]interface{}
	writes map[string]interface{}
	nWrite int
}

func newFake() *fakeDriver {
	return &fakeDriver{vals: map[string]interface{}{}, writes: map[string]interface{}{}}
}

func (f *fakeDriver) Connect() error { return nil }
func (f *fakeDriver) Close() error   { return nil }
func (f *fakeDriver) ReadPoint(p TagPoint) (interface{}, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vals[p.Address], nil
}
func (f *fakeDriver) WritePoint(p TagPoint, v interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.writes[p.Address] = v
	f.nWrite++
	return nil
}

// TestPollerReadsIntoTags verifies a poll copies each point's device value into
// its tag.
func TestPollerReadsIntoTags(t *testing.T) {
	f := newFake()
	f.vals["40001"] = int16(42)
	db := core.NewTagDB()
	pts := []TagPoint{{Tag: "level", Address: "40001", Type: TypeInt16, Access: ReadOnly}}
	NewPoller(f, pts, db, time.Hour).PollOnce()

	if got := db.GetOrCreate("level", core.Meta{}).Value().Float(); got != 42 {
		t.Errorf("level = %v, want 42", got)
	}
}

// TestPollerEchoGuard verifies the poll→tag update does NOT echo back as a
// device write, while a genuine tag edit does write once.
func TestPollerEchoGuard(t *testing.T) {
	f := newFake()
	f.vals["a"] = int16(7)
	db := core.NewTagDB()
	pts := []TagPoint{{Tag: "sp", Address: "a", Type: TypeInt16, Order: BigEndian, Access: ReadWrite}}
	p := NewPoller(f, pts, db, time.Hour)
	p.wireWrites()

	p.PollOnce() // device -> tag; must not write back
	f.mu.Lock()
	n0 := f.nWrite
	f.mu.Unlock()
	if n0 != 0 {
		t.Fatalf("poll echoed as %d device writes, want 0", n0)
	}

	db.GetOrCreate("sp", core.Meta{}).SetValue(99.0) // user edit -> one device write
	f.mu.Lock()
	n1, wrote := f.nWrite, f.writes["a"]
	f.mu.Unlock()
	if n1 != 1 {
		t.Errorf("user edit produced %d writes, want 1", n1)
	}
	if asFloat(wrote) != 99 {
		t.Errorf("device write = %v, want 99", wrote)
	}
}

// TestPollerReadOnlyNeverWrites confirms a read-only point ignores tag edits.
func TestPollerReadOnlyNeverWrites(t *testing.T) {
	f := newFake()
	db := core.NewTagDB()
	pts := []TagPoint{{Tag: "ro", Address: "b", Type: TypeInt16, Access: ReadOnly}}
	p := NewPoller(f, pts, db, time.Hour)
	p.wireWrites()

	db.GetOrCreate("ro", core.Meta{}).SetValue(5.0)
	f.mu.Lock()
	n := f.nWrite
	f.mu.Unlock()
	if n != 0 {
		t.Errorf("read-only point wrote %d times, want 0", n)
	}
}
