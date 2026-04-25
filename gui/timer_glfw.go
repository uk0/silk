//go:build !windows

package gui

import (
	"sync"
	"time"
)

var (
	timerMu    sync.Mutex
	timerMap   = make(map[uintptr]*timerEntry)
	timerNextId uintptr
)

type timerEntry struct {
	interval time.Duration
	callback func()
	lastFire time.Time
}

// Timer is a low-precision timer for the UI thread
type Timer uintptr

func (t *Timer) Stop() {
	timerMu.Lock()
	defer timerMu.Unlock()
	if *t != 0 {
		delete(timerMap, uintptr(*t))
		*t = 0
	}
}

func (t *Timer) Start(millisecond uint32, f func()) bool {
	t.Stop()
	timerMu.Lock()
	defer timerMu.Unlock()
	timerNextId++
	id := timerNextId
	timerMap[id] = &timerEntry{
		interval: time.Duration(millisecond) * time.Millisecond,
		callback: f,
		lastFire: time.Now(),
	}
	*t = Timer(id)
	return true
}

// processTimers is called from MainLoop to fire due timers
func processTimers() {
	timerMu.Lock()
	// Collect due timers
	var due []func()
	now := time.Now()
	for _, entry := range timerMap {
		if now.Sub(entry.lastFire) >= entry.interval {
			entry.lastFire = now
			due = append(due, entry.callback)
		}
	}
	timerMu.Unlock()

	// Fire outside of lock
	for _, cb := range due {
		func() {
			defer func() {
				recover()
			}()
			cb()
		}()
	}
}
