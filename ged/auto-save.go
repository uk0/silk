package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"os"
	"time"
)

// AutoSaver provides automatic periodic saving of the current scene
// to a recovery file ({filename}.autosave). If the application crashes
// or is closed without saving, the autosave file can be used for recovery.
type AutoSaver struct {
	enabled  bool
	interval time.Duration
	timer    gui.Timer
	scene    *GedScene
	lastSave time.Time
}

// GlobalAutoSaver is the singleton auto-saver instance.
var GlobalAutoSaver *AutoSaver

// NewAutoSaver creates a new auto-saver with a default 60-second interval.
func NewAutoSaver() *AutoSaver {
	return &AutoSaver{
		enabled:  true,
		interval: 60 * time.Second,
	}
}

// SetEnabled enables or disables auto-saving.
func (this *AutoSaver) SetEnabled(enabled bool) {
	this.enabled = enabled
	if !enabled {
		this.Stop()
	}
}

// IsEnabled returns whether auto-saving is enabled.
func (this *AutoSaver) IsEnabled() bool {
	return this.enabled
}

// SetInterval sets the auto-save interval duration.
func (this *AutoSaver) SetInterval(d time.Duration) {
	this.interval = d
	// Restart with new interval if already running
	if this.scene != nil && this.enabled {
		this.Stop()
		this.Start(this.scene)
	}
}

// Start begins auto-saving the given scene at the configured interval.
func (this *AutoSaver) Start(scene *GedScene) {
	this.scene = scene
	this.lastSave = time.Now()

	if !this.enabled {
		return
	}

	ms := uint32(this.interval.Milliseconds())
	if ms < 1000 {
		ms = 1000
	}

	this.timer.Start(ms, func() {
		this.tick()
	})
}

// Stop halts the auto-save timer.
func (this *AutoSaver) Stop() {
	this.timer.Stop()
}

// tick is called by the timer. It checks if the scene has been modified
// and saves to the autosave file if needed.
func (this *AutoSaver) tick() {
	if !this.enabled || this.scene == nil {
		return
	}

	// Only save if the scene has unsaved changes (undo stack is not clean)
	if this.scene.IsClean() {
		return
	}

	filename := this.scene.Filename()
	if filename == "" {
		return
	}

	autosavePath := filename + ".autosave"

	doc := this.scene.SaveDesign()
	err := doc.SaveFile(autosavePath)
	if err != nil {
		core.Error(err)
		return
	}

	this.lastSave = time.Now()
	core.Debug("AutoSave: saved to", autosavePath)
}

// CheckRecovery checks if an autosave file exists for the given filename.
// Returns the autosave path if it exists and is newer than the original file,
// or empty string if no recovery is available.
func CheckRecovery(filename string) string {
	if filename == "" {
		return ""
	}
	autosavePath := filename + ".autosave"
	info, err := os.Stat(autosavePath)
	if err != nil {
		return ""
	}

	// Check if autosave is newer than the original file
	origInfo, err := os.Stat(filename)
	if err != nil {
		// Original doesn't exist but autosave does -- recovery available
		return autosavePath
	}

	if info.ModTime().After(origInfo.ModTime()) {
		return autosavePath
	}
	return ""
}

// CleanupAutosave removes the autosave file for the given filename.
func CleanupAutosave(filename string) {
	if filename == "" {
		return
	}
	autosavePath := filename + ".autosave"
	os.Remove(autosavePath)
}

// InitAutoSaver creates and initializes the global auto-saver.
func InitAutoSaver() {
	if GlobalAutoSaver == nil {
		GlobalAutoSaver = NewAutoSaver()
	}
}
