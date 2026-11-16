package main

import (
	"sync/atomic"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
)

// Pure shared state: no scene, path callbacks or UI back-reference. A serialized
// physical writer advances this baseline even if UI adoption was retired. The
// next queued save then checks our last actual write, not an obsolete UI clean
// marker. An external change still fails the atomic writer's content check.
type saveDiskState struct {
	version     atomic.Pointer[core.FileFingerprint]
	initialText atomic.Pointer[string]
}

func newSaveDiskState(v core.FileFingerprint) *saveDiskState {
	s := new(saveDiskState)
	s.store(v)
	return s
}
func newSourceDiskState(text string) *saveDiskState {
	s := new(saveDiskState)
	s.initialText.Store(&text)
	return s
}

// Called by the physical worker: hashing a large saved source baseline must
// not become new UI work at admission. Drop the retained source after hashing.
func (s *saveDiskState) load() core.FileFingerprint {
	if v := s.version.Load(); v != nil {
		return *v
	}
	text := s.initialText.Load()
	if text == nil {
		if v := s.version.Load(); v != nil {
			return *v
		}
		return core.FileFingerprint{}
	}
	v := core.FingerprintBytes([]byte(*text))
	s.version.CompareAndSwap(nil, &v)
	s.initialText.Store(nil)
	return *s.version.Load()
}
func (s *saveDiskState) store(v core.FileFingerprint) { s.version.Store(&v); s.initialText.Store(nil) }

// UI-only binding of a physical baseline to a currently installed scene/path.
type designDiskBinding struct {
	scene *ged.GedScene
	path  string
	state *saveDiskState
}

func bindDesignDisk(scene *ged.GedScene, path string, v core.FileFingerprint) {
	w := globalWorkbench
	if w == nil || w.canvas == nil || w.canvas.GedScene() != scene || workspaceSearchKey(scene.Filename()) != workspaceSearchKey(path) {
		return
	}
	w.designDisk = designDiskBinding{scene: scene, path: workspaceSearchKey(path), state: newSaveDiskState(v)}
	if c := w.designSaves; c != nil {
		c.baselines = nil
	}
}

func (c *ownedDesignSaver) diskState(scene *ged.GedScene, path string) *saveDiskState {
	key := workspaceSearchKey(path)
	if c.baselineScene != scene {
		c.baselines = nil
		c.baselineScene = scene
	}
	if c.baselines == nil {
		c.baselines = make(map[string]*saveDiskState)
	}
	if state := c.baselines[key]; state != nil {
		return state
	}
	state := newSaveDiskState(core.MissingFingerprint())
	if b := c.owner.designDisk; b.scene == scene && b.path == key && b.state != nil {
		state = b.state
	}
	// A new/unknown existing target needs explicit conflict review, not silent
	// overwrite. Known-absent is fail closed until that UI review is wired.
	if len(c.baselines) >= 32 {
		c.baselines = make(map[string]*saveDiskState)
	}
	c.baselines[key] = state
	return state
}
