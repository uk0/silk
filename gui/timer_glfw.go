//go:build !windows

package gui

import (
	"sync"
	"time"
)

var (
	// timerMu guards the map only. It does not make Timer callable from a
	// background goroutine: the Win32 backend cannot honour that (see
	// timer_windows.go), so off-thread callers must still go through Post().
	timerMu  sync.Mutex
	timerMap = make(map[uintptr]*timerEntry)
	// timerNextId only ever counts up, so an id names one arming of one Timer
	// and presence in timerMap is identity.
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
	var due []uintptr
	now := time.Now()
	for id, entry := range timerMap {
		if now.Sub(entry.lastFire) >= entry.interval {
			entry.lastFire = now
			due = append(due, id)
		}
	}
	timerMu.Unlock()

	// Fire outside of lock, re-reading each id: a callback earlier in this
	// batch may have stopped a timer later in it, and Stop() has to mean no
	// further callback. Win32 gets that for free — it looks the id up in
	// timerMap as each WM_TIMER is dispatched.
	for _, id := range due {
		timerMu.Lock()
		entry := timerMap[id]
		timerMu.Unlock()
		if entry == nil {
			continue
		}
		func() {
			defer func() {
				recover()
			}()
			entry.callback()
		}()
	}
}
