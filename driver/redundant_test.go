package driver

import (
	"fmt"
	"testing"
)

// redFake is an in-memory Driver for the redundancy tests. Its fail toggle makes
// Connect/ReadPoint/WritePoint error on demand, and val is the canned read
// value so a test can tell which side answered. It is exercised synchronously,
// so it needs no mutex (unlike driver_test.go's fakeDriver).
type redFake struct {
	name   string
	val    interface{}
	fail   bool
	nRead  int
	nWrite int
	last   interface{} // last value accepted by WritePoint
}

func (f *redFake) Connect() error { return f.errf("connect") }
func (f *redFake) Close() error   { return nil }

func (f *redFake) ReadPoint(p TagPoint) (interface{}, error) {
	f.nRead++
	if f.fail {
		return nil, f.errf("read")
	}
	return f.val, nil
}

func (f *redFake) WritePoint(p TagPoint, v interface{}) error {
	f.nWrite++
	if f.fail {
		return f.errf("write")
	}
	f.last = v
	return nil
}

func (f *redFake) errf(op string) error {
	if f.fail {
		return fmt.Errorf("%s: %s fail", f.name, op)
	}
	return nil
}

// TestRedundantReadsPrimary: both devices healthy -> reads come from the primary
// and never touch the backup.
func TestRedundantReadsPrimary(t *testing.T) {
	p := &redFake{name: "p", val: int16(11)}
	b := &redFake{name: "b", val: int16(22)}
	r := NewRedundant(p, b)
	if err := r.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	v, err := r.ReadPoint(TagPoint{Address: "x"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if v != int16(11) {
		t.Errorf("read = %v, want primary 11", v)
	}
	if r.Active() != "primary" {
		t.Errorf("Active = %q, want primary", r.Active())
	}
	if b.nRead != 0 {
		t.Errorf("backup read %d times, want 0", b.nRead)
	}
}

// TestRedundantReadFailover: the primary goes down after connect -> the read
// fails over to the backup's value and the backup becomes active.
func TestRedundantReadFailover(t *testing.T) {
	p := &redFake{name: "p", val: int16(11)}
	b := &redFake{name: "b", val: int16(22)}
	r := NewRedundant(p, b)
	if err := r.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	p.fail = true // primary fails after a healthy connect
	v, err := r.ReadPoint(TagPoint{Address: "x"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if v != int16(22) {
		t.Errorf("failover read = %v, want backup 22", v)
	}
	if r.Active() != "backup" {
		t.Errorf("Active = %q, want backup after failover", r.Active())
	}
}

// TestRedundantFlipsBack: primary fails (-> backup), then the backup fails while
// the primary recovers -> traffic flips back to the primary.
func TestRedundantFlipsBack(t *testing.T) {
	p := &redFake{name: "p", val: int16(11)}
	b := &redFake{name: "b", val: int16(22)}
	r := NewRedundant(p, b)
	if err := r.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	p.fail = true // primary down -> fail over to backup
	if _, err := r.ReadPoint(TagPoint{Address: "x"}); err != nil {
		t.Fatalf("read after primary fail: %v", err)
	}
	if r.Active() != "backup" {
		t.Fatalf("Active = %q, want backup", r.Active())
	}

	b.fail = true // backup down, primary back up -> flip back
	p.fail = false
	v, err := r.ReadPoint(TagPoint{Address: "x"})
	if err != nil {
		t.Fatalf("read after backup fail: %v", err)
	}
	if v != int16(11) {
		t.Errorf("recovered read = %v, want primary 11", v)
	}
	if r.Active() != "primary" {
		t.Errorf("Active = %q, want primary after flip back", r.Active())
	}
}

// TestRedundantReadBothFail: with both devices down a read returns the standby's
// (second) error rather than the active one.
func TestRedundantReadBothFail(t *testing.T) {
	p := &redFake{name: "p", fail: true}
	b := &redFake{name: "b", fail: true}
	r := NewRedundant(p, b)
	_, err := r.ReadPoint(TagPoint{Address: "x"})
	if err == nil {
		t.Fatal("read with both down: want error, got nil")
	}
	if err.Error() != "b: read fail" {
		t.Errorf("err = %v, want the backup's read error", err)
	}
}

// TestRedundantWriteFailover: a failed primary write falls over to the backup,
// which records the value and becomes active.
func TestRedundantWriteFailover(t *testing.T) {
	p := &redFake{name: "p"}
	b := &redFake{name: "b"}
	r := NewRedundant(p, b)
	if err := r.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	p.fail = true // primary write fails
	if err := r.WritePoint(TagPoint{Address: "x"}, int16(7)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if r.Active() != "backup" {
		t.Errorf("Active = %q, want backup after write failover", r.Active())
	}
	if b.last != int16(7) {
		t.Errorf("backup last write = %v, want 7", b.last)
	}
}

// TestRedundantConnectOneUp: connect succeeds when only one device comes up, and
// starts on whichever side is actually connected.
func TestRedundantConnectOneUp(t *testing.T) {
	// Primary down, backup up -> start on the backup.
	p := &redFake{name: "p", fail: true}
	b := &redFake{name: "b", val: int16(1)}
	r := NewRedundant(p, b)
	if err := r.Connect(); err != nil {
		t.Fatalf("connect with backup up: %v", err)
	}
	if r.Active() != "backup" {
		t.Errorf("Active = %q, want backup when primary down", r.Active())
	}

	// Primary up, backup down -> stay on the primary.
	p2 := &redFake{name: "p", val: int16(1)}
	b2 := &redFake{name: "b", fail: true}
	r2 := NewRedundant(p2, b2)
	if err := r2.Connect(); err != nil {
		t.Fatalf("connect with primary up: %v", err)
	}
	if r2.Active() != "primary" {
		t.Errorf("Active = %q, want primary when backup down", r2.Active())
	}
}

// TestRedundantConnectBothFail: connect errors only when both devices fail.
func TestRedundantConnectBothFail(t *testing.T) {
	p := &redFake{name: "p", fail: true}
	b := &redFake{name: "b", fail: true}
	r := NewRedundant(p, b)
	if err := r.Connect(); err == nil {
		t.Fatal("connect with both down: want error, got nil")
	}
}
