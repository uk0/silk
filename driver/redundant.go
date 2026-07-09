package driver

import (
	"fmt"
	"sync"
)

// Redundant pairs a primary and a backup Driver behind the Driver interface and
// fails over between them automatically, the way FameView (杰控) runs 百毫秒级
// device redundancy: one device carries traffic while the other stands by, and a
// fault on the active side moves traffic to the standby without the caller
// noticing.
//
// Failover is driven by operation results, not a background health poll (v1):
// every ReadPoint/WritePoint tries the currently active driver first and, on
// error, retries once on the standby. A successful retry promotes the standby to
// active, so the next call goes straight to the side that is actually working.
type Redundant struct {
	primary, backup Driver
	mu              sync.Mutex
	usingBackup     bool // guarded by mu; false => primary is active
}

var _ Driver = (*Redundant)(nil)

// NewRedundant builds a redundant driver over primary and backup. primary is the
// preferred device; backup takes over on primary failure.
func NewRedundant(primary, backup Driver) *Redundant {
	return &Redundant{primary: primary, backup: backup}
}

// Connect connects both devices and succeeds if at least one comes up, recording
// which side is active: primary when it connects, otherwise the backup. It fails
// only if both devices fail to connect.
func (r *Redundant) Connect() error {
	perr := r.primary.Connect()
	berr := r.backup.Connect()

	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case perr == nil:
		r.usingBackup = false // prefer the primary whenever it is up
	case berr == nil:
		r.usingBackup = true // primary down, run on the backup
	default:
		return fmt.Errorf("driver: redundant: both devices failed to connect: primary: %v; backup: %v", perr, berr)
	}
	return nil
}

// Close closes both devices, returning the first error seen (primary before
// backup); both are always closed regardless of either result.
func (r *Redundant) Close() error {
	err := r.primary.Close()
	if berr := r.backup.Close(); err == nil {
		err = berr
	}
	return err
}

// ReadPoint reads p from the active device; on error it retries once on the
// standby and, if that succeeds, promotes the standby to active. It returns the
// value from whichever device answered, or the standby's error if both fail.
func (r *Redundant) ReadPoint(p TagPoint) (interface{}, error) {
	active, standby, usingBackup := r.pick()
	if v, err := active.ReadPoint(p); err == nil {
		return v, nil
	}
	v, err := standby.ReadPoint(p)
	if err != nil {
		return nil, err
	}
	r.promote(usingBackup)
	return v, nil
}

// WritePoint writes v to p on the active device; on error it retries once on the
// standby and, if that succeeds, promotes the standby to active. It returns nil
// if either device accepted the write, or the standby's error if both fail.
func (r *Redundant) WritePoint(p TagPoint, v interface{}) error {
	active, standby, usingBackup := r.pick()
	if err := active.WritePoint(p, v); err == nil {
		return nil
	}
	if err := standby.WritePoint(p, v); err != nil {
		return err
	}
	r.promote(usingBackup)
	return nil
}

// Active reports which device is currently carrying traffic, "primary" or
// "backup", for status display.
func (r *Redundant) Active() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usingBackup {
		return "backup"
	}
	return "primary"
}

// pick returns the active driver, its standby, and the failover state they were
// chosen under so a successful retry can promote the right side.
func (r *Redundant) pick() (active, standby Driver, usingBackup bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.usingBackup {
		return r.backup, r.primary, true
	}
	return r.primary, r.backup, false
}

// promote records that the standby is now active after a successful failover.
// wasBackup is the state observed by the matching pick, so the new active side
// is its opposite; writing an absolute target (not a blind toggle) keeps
// concurrent failovers idempotent.
func (r *Redundant) promote(wasBackup bool) {
	r.mu.Lock()
	r.usingBackup = !wasBackup
	r.mu.Unlock()
}
