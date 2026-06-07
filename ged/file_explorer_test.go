package ged

import "testing"

// guaranteedIcons is the set of resource icon names the file tree is allowed
// to reference. Every one resolves to a real PNG (and the expanders also have
// procedural fallbacks), so a row can never render the red-X missing-icon
// placeholder. iconNameForEntry must never return a name outside this set.
var guaranteedIcons = map[string]bool{
	"folder":             true,
	"document":           true,
	"expander-collapsed": true,
	"expander-expanded":  true,
}

// TestIconNameForEntry checks the pure extension/dir -> icon-name mapping that
// the Draw path uses. Directories map to "folder"; every file maps to
// "document" regardless of extension (per-extension glyphs are a follow-up).
func TestIconNameForEntry(t *testing.T) {
	cases := []struct {
		name  string
		isDir bool
		ext   string
		want  string
	}{
		{"directory", true, "", "folder"},
		{"directory ignores ext", true, ".go", "folder"},
		{"go file", false, ".go", "document"},
		{"mod file", false, ".mod", "document"},
		{"markdown file", false, ".md", "document"},
		{"text file", false, ".txt", "document"},
		{"no extension", false, "", "document"},
		{"unknown extension", false, ".xyz", "document"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := iconNameForEntry(c.isDir, c.ext)
			if got != c.want {
				t.Fatalf("iconNameForEntry(%v, %q) = %q, want %q", c.isDir, c.ext, got, c.want)
			}
			if !guaranteedIcons[got] {
				t.Fatalf("iconNameForEntry(%v, %q) returned %q, which is not a guaranteed-present icon", c.isDir, c.ext, got)
			}
		})
	}
}
