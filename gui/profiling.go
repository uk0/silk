package gui

import (
	"fmt"
	"os"
	"runtime"
	"runtime/pprof"
	"time"
)

// StartCPUProfile begins recording a Go pprof CPU profile to filename.
// Call StopCPUProfile() to stop and flush the profile to disk. Calling
// StartCPUProfile while a profile is already running returns an error from
// runtime/pprof.
func StartCPUProfile(filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	if err := pprof.StartCPUProfile(f); err != nil {
		f.Close()
		return err
	}
	return nil
}

// StopCPUProfile stops the current CPU profile and flushes any buffered data.
// Safe to call when no profile is active (no-op).
func StopCPUProfile() {
	pprof.StopCPUProfile()
}

// WriteHeapProfile triggers a GC and writes the current heap profile to
// filename. Use for one-shot allocation snapshots.
func WriteHeapProfile(filename string) error {
	runtime.GC()
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	return pprof.WriteHeapProfile(f)
}

// MemStats returns a one-line human-readable summary of the current process
// memory state (live allocation, lifetime allocation, OS-reserved, GC count,
// goroutine count). Useful for inline logging or perf overlays.
func MemStats() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return fmt.Sprintf("Alloc=%dKB TotalAlloc=%dMB Sys=%dMB NumGC=%d Goroutines=%d",
		m.Alloc/1024, m.TotalAlloc/1024/1024, m.Sys/1024/1024, m.NumGC,
		runtime.NumGoroutine())
}

// MemStatsDetailed returns a multi-line dump with additional fields useful
// when chasing leaks: heap objects, mspan/mcache, GC pause stats.
func MemStatsDetailed() string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	var lastGCMs uint64
	if m.LastGC != 0 {
		lastGCMs = (uint64(time.Now().UnixNano()) - m.LastGC) / 1_000_000
	}
	return fmt.Sprintf(
		"Alloc=%dKB\nTotalAlloc=%dMB\nSys=%dMB\nHeapObjects=%d\nHeapInuse=%dKB\nStackInuse=%dKB\nNumGC=%d\nLastGC=%dms ago\nGoroutines=%d",
		m.Alloc/1024,
		m.TotalAlloc/1024/1024,
		m.Sys/1024/1024,
		m.HeapObjects,
		m.HeapInuse/1024,
		m.StackInuse/1024,
		m.NumGC,
		lastGCMs,
		runtime.NumGoroutine(),
	)
}
