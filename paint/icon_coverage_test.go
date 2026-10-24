package paint

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// Static shell icons are shipped UI contracts. Widget SetIconName properties,
// user templates, plugin ViewDef.Icon values and custom application icons are
// dynamic resources: they are intentionally not restricted to built-in names.
// Parse syntax rather than grep so comments and generated Go string literals
// cannot accidentally make this coverage test green.
func staticShellIconReferences(file *ast.File) map[string][]token.Pos {
	refs := make(map[string][]token.Pos)
	add := func(expr ast.Expr) {
		lit, ok := expr.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return
		}
		name, err := strconv.Unquote(lit.Value)
		if err == nil && name != "" {
			refs[name] = append(refs[name], lit.Pos())
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.CallExpr:
			var name string
			switch fn := node.Fun.(type) {
			case *ast.Ident:
				name = fn.Name
			case *ast.SelectorExpr:
				name = fn.Sel.Name
			}
			arg := 0
			switch name {
			case "LoadIcon", "NewStatusIconLabel", "AddIconLabel":
			case "addIconAction":
				arg = 1
			default:
				return true
			}
			if len(node.Args) > arg {
				add(node.Args[arg])
			}
		case *ast.KeyValueExpr:
			if key, ok := node.Key.(*ast.Ident); ok && key.Name == "Icon" {
				add(node.Value)
			}
		}
		return true
	})
	return refs
}

func TestStaticShellIconReferencesIgnoreUserAndCommentData(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package fixture
	// LoadIcon("comment-is-not-an-icon")
	var _ = ViewDef{Icon: "window"}
	func demo(userName string) {
		paint.LoadIcon("error")
		addIconAction("Label", "save", "Tooltip", nil)
		button.SetIconName("custom-widget-resource")
		paint.LoadIcon(userName)
		_ = "LoadIcon(\"generated-code-not-a-call\")"
	}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	refs := staticShellIconReferences(file)
	if len(refs) != 3 || refs["window"] == nil || refs["error"] == nil || refs["save"] == nil {
		t.Fatalf("static/dynamic icon boundaries were lost: %v", refs)
	}
}

func TestIDEShellStaticIconReferencesHaveBuiltins(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate repository for source coverage")
	}
	root := filepath.Dir(filepath.Dir(file))
	fset := token.NewFileSet()
	names := make(map[string]bool)
	for _, dir := range []string{"cmd/silkide", "ged", "gui", "graph", "prop"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, filepath.Join(root, dir, entry.Name()), nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			for name, refs := range staticShellIconReferences(file) {
				names[name] = true
				if proceduralFallbacks[name] == nil {
					for _, ref := range refs {
						t.Errorf("%s: shell icon %q has no vector fallback (would show image-missing without an external resource pack)", fset.Position(ref), name)
					}
				}
			}
		}
	}
	if len(names) < 50 {
		t.Fatalf("source coverage unexpectedly shrank to %d icon names", len(names))
	}
	// Exercise the real loader with no external resource pack or warm cache.
	cache, sources, preference := iconCache, iconSrcCache, preferVectorIcons
	defer func() { iconCache, iconSrcCache, preferVectorIcons = cache, sources, preference }()
	iconCache = make(map[string]*cachedIcon)
	iconSrcCache = make(map[string]*iconSrc)
	preferVectorIcons = true
	for name := range names {
		got, ok := LoadIcon(name).(*icon)
		if !ok || got.name != name || got.draw == nil {
			t.Errorf("%q resolved to a missing or non-vector icon", name)
		}
	}
	t.Logf("checked %d distinct static shell icon names across IDE, designer, GUI, graph and property panels", len(names))
}

var designerPanelIconNames = []string{
	"error", "question", "help", "window", "form", "link", "keymap", "test-explorer",
	"call-hierarchy", "type-hierarchy", "project", "diagram", "merge", "maximize", "restore",
	"layout-horizontal", "layout-vertical", "layout-grid", "layout-form", "layout-break", "layout-adjust",
}

func TestDesignerPanelIconsRenderExactPaletteAndDeviceScale(t *testing.T) {
	previous := CurrentIconPalette()
	defer SetIconPalette(previous)
	for _, palette := range []IconPalette{LightIconPalette(), DarkIconPalette()} {
		SetIconPalette(palette)
		for _, name := range designerPanelIconNames {
			t.Run(name, func(t *testing.T) {
				drawer := proceduralFallbacks[name]
				if drawer == nil {
					t.Fatal("missing icon drawer")
				}
				ico := genProceduralIcon(name, drawer)
				semantic := palette.Accent
				switch name {
				case "error":
					semantic = palette.Danger
				case "project", "layout-break":
					semantic = palette.Warning
				case "test-explorer":
					semantic = palette.Success
				case "type-hierarchy", "merge":
					semantic = palette.Violet
				case "keymap", "maximize", "restore":
					semantic = palette.Foreground
				}
				for _, size := range []int{16, 20, 24, 32, 48, 64} {
					device := NewPixmap(size, size)
					p := device.NewPainter().(*cairoPainter)
					p.Scale(float64(size)/16, float64(size)/16)
					p.DrawIcon(ico, 16, false)
					got, err := device.Image()
					if err != nil {
						t.Fatal(err)
					}
					want, err := ico.Pixmap(size).Image()
					if err != nil {
						t.Fatal(err)
					}
					assertIconImagesEqual(t, size, got, want)
					painted, exactSemantic := 0, 0
					for y := 0; y < size; y++ {
						for x := 0; x < size; x++ {
							pixel := ColorModel.Convert(got.At(x, y)).(Color)
							if pixel.A > 0 {
								painted++
							}
							if pixel == semantic {
								exactSemantic++
							}
						}
					}
					if painted == 0 {
						t.Fatalf("%dpx glyph is blank", size)
					}
					// Overlapping antialiased strokes legitimately blend tokens;
					// assert exact semantic interior pixels, not every AA edge.
					if size == 64 && exactSemantic == 0 {
						t.Fatalf("glyph lost its exact sRGB semantic color %v", semantic)
					}
					for _, corner := range [][2]int{{0, 0}, {size - 1, 0}, {0, size - 1}, {size - 1, size - 1}} {
						_, _, _, alpha := got.At(corner[0], corner[1]).RGBA()
						if alpha != 0 {
							t.Fatalf("%dpx icon lost transparent padding at %v", size, corner)
						}
					}
				}
			})
		}
	}
}
