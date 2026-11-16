package main

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
)

type saveReservationOwner struct{ marker byte } // pure opaque identity
type saveReservation struct {
	owner                *saveReservationOwner
	paths                []string
	physicalDone, uiDone bool // guarded by saveReservations
}

var saveReservations = struct {
	sync.Mutex
	active  map[*saveReservation]bool
	changed chan struct{}
}{active: make(map[*saveReservation]bool), changed: make(chan struct{})}

func savePathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator)) || strings.HasPrefix(b, a+string(filepath.Separator))
}

func reserveSavePaths(owner *saveReservationOwner, paths []string) (*saveReservation, bool) {
	t := &saveReservation{owner: owner}
	for _, p := range paths {
		t.paths = append(t.paths, workspaceSearchKey(p))
	}
	saveReservations.Lock()
	defer saveReservations.Unlock()
	for active := range saveReservations.active {
		if owner != nil && active.owner == owner {
			continue
		}
		for _, a := range active.paths {
			for _, b := range t.paths {
				if savePathsOverlap(a, b) {
					return nil, false
				}
			}
		}
	}
	saveReservations.active[t] = true
	return t, true
}

// Release a ticket only after BOTH physical completion and UI publication or
// explicit retirement. A canceled/closed owner cannot release blocked OS work;
// a fast worker cannot permit rename/quit before queued UI publication.
func (t *saveReservation) finish(physical bool) {
	if t == nil {
		return
	}
	saveReservations.Lock()
	defer saveReservations.Unlock()
	if physical {
		t.physicalDone = true
	} else {
		t.uiDone = true
	}
	if saveReservations.active[t] && t.physicalDone && t.uiDone {
		delete(saveReservations.active, t)
		close(saveReservations.changed)
		saveReservations.changed = make(chan struct{})
	}
}

// Outside UI only. Notifications, not polling, cover both actual physical work
// and its pending UI publication. Cancellation never removes a reservation.
func waitSaveReservations(ctx context.Context) error {
	for {
		saveReservations.Lock()
		if len(saveReservations.active) == 0 {
			saveReservations.Unlock()
			return nil
		}
		changed := saveReservations.changed
		saveReservations.Unlock()
		select {
		case <-changed:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

func anySaveBusy() bool {
	saveReservations.Lock()
	defer saveReservations.Unlock()
	return len(saveReservations.active) != 0
}
func anySaveBusyExcept(owner *saveReservationOwner) bool {
	saveReservations.Lock()
	defer saveReservations.Unlock()
	for t := range saveReservations.active {
		if t.owner != owner {
			return true
		}
	}
	return false
}
func savePathBusy(path string) bool {
	key := workspaceSearchKey(path)
	if key == "" {
		return false
	}
	saveReservations.Lock()
	defer saveReservations.Unlock()
	for t := range saveReservations.active {
		for _, p := range t.paths {
			if savePathsOverlap(key, p) {
				return true
			}
		}
	}
	return false
}
func designSavePaths(path string) []string {
	stem := strings.TrimSuffix(path, ".silkui")
	return []string{path, path + ".autosave", stem + ".silk.go", stem + ".go"}
}
