// Package fswatch is Silk's filesystem-watcher runtime — the
// equivalent of Qt's QFileSystemWatcher. Apps register paths (files
// or directories) via AddPath; the Watcher polls them on a background
// goroutine and delivers Event values on its Events channel when a
// file is created, modified, or removed.
//
// Why polling and not fsnotify / inotify / FSEvents? Two reasons:
//
//  1. No third-party deps. Silk's design philosophy is "pure Go +
//     CGO only for Cairo / GLFW / SQLite". A native event API would
//     pull in fsnotify (with its own per-OS C bindings), which we
//     can't justify for the typical UI use case (config files, theme
//     reload, IDE auto-rebuild on save).
//  2. Determinism. Polling sees coalesced events — a file rapidly
//     written multiple times surfaces as a single Modified event,
//     which is what hosts usually want anyway. Native event APIs
//     produce one event per write and the host has to debounce
//     manually.
//
// Cost: PollInterval-bounded latency for change detection. The
// default 500ms is fast enough for hot reload + slow enough that
// polling cost is negligible (a stat call per watched path per
// 500ms).
//
// Typical usage:
//
//	w := fswatch.New()
//	defer w.Stop()
//	w.AddPath("/path/to/config.json")
//	w.AddPath("/path/to/templates")  // directory
//
//	go func() {
//	    for ev := range w.Events() {
//	        switch ev.Kind {
//	        case fswatch.Modified:
//	            reloadConfig(ev.Path)
//	        case fswatch.Created:
//	            indexNewTemplate(ev.Path)
//	        case fswatch.Removed:
//	            forgetFile(ev.Path)
//	        }
//	    }
//	}()
//
// The Events channel is buffered (default 64); slow consumers risk
// dropped events and will see a gap in the sequence. Hosts that
// can't keep up should drain on a dedicated goroutine and forward to
// their own buffered queue.
package fswatch
