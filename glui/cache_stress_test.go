package glui

import (
	"testing"
)

// TestCacheStressEviction is reserved for a real GL-context integration
// test that uploads many real GL textures and verifies that LRU eviction
// reclaims VRAM under realistic load. Without a current GL context the
// CairoCompat upload paths short-circuit early (see uploadPixmapNoCache,
// which returns nil when r.ctx is nil), so a test that runs inside the
// off-GL test harness cannot exercise the GL-side eviction we care about.
//
// The unit tests in cairo_compat_test.go already cover the *eviction
// logic* without needing GL textures by populating the cache with mock
// pixmapEntry values directly. See:
//   - TestCacheBeginFrameEvictsStaleEntries
//   - TestCacheBeginFramePreservesRecent
//   - TestCacheBeginFrameEarlyFramesNoEvict
//   - TestCacheHardCapEvictsOldest25Pct
//   - TestCacheHardCapAtBoundary
//   - TestCacheHardCapIcon
//   - TestCacheBindRendererPreservesFrameCount
//
// Those exercises the LRU clock, hard-cap, and BindRenderer survival
// paths. The remaining work is purely "does Texture.Free actually
// reclaim driver VRAM?" — that is best validated by an integration
// harness with a real Window, not by a unit test.
func TestCacheStressEviction(t *testing.T) {
	t.Skip("integration test — requires a real GL context for VRAM reclamation; cache logic itself is covered by the TestCacheBeginFrame* and TestCacheHardCap* unit tests in cairo_compat_test.go")
}
