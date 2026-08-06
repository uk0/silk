package ged

import (
	"strings"
	"testing"

	"github.com/uk0/silk/core"
)

// TestGoMetaLiteralOmitsZeroFields: the emitted literal names only the fields the
// designer actually filled in. core.Meta's zero value arms no alarms
// (core.limitSet), so writing the zeros back would be noise, not information.
func TestGoMetaLiteralOmitsZeroFields(t *testing.T) {
	got := goMetaLiteral(core.Meta{Unit: "bar", Max: 10, Hi: 8, Desc: `压力"高"`})
	want := `core.Meta{Unit: "bar", Max: 10, Hi: 8, Desc: "压力\"高\""}`
	if got != want {
		t.Errorf("goMetaLiteral =\n  %s\nwant\n  %s", got, want)
	}

	if got := goMetaLiteral(core.Meta{}); got != "core.Meta{}" {
		t.Errorf("goMetaLiteral(zero) = %s, want core.Meta{}", got)
	}
	if got := goMetaLiteral(core.Meta{Min: -40, LoLo: -30}); got != "core.Meta{Min: -40, LoLo: -30}" {
		t.Errorf("goMetaLiteral = %s, want core.Meta{Min: -40, LoLo: -30}", got)
	}
}

// TestGenerateCodeSeedsDeclaredTags proves the declared dictionary reaches the
// generated app: a design that declares one tag is a SCADA design even with no
// widget bound to it, and BindServices registers the declaration — name and Meta
// — BEFORE scada.BindScreen. Order is the whole point: GetOrCreate applies Meta
// only when it creates the tag, so a bind that ran first would leave the tag
// permanently stuck with an empty Meta and no alarm limits. vetGeneratedCode then
// type-checks the result against the real core / scada API.
func TestGenerateCodeSeedsDeclaredTags(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping compile test in short mode")
	}

	scene := NewGedScene()
	scene.SetFormTitle("Dict")
	scene.SetSize(160, 120)
	scene.SetTagDict([]TagDecl{
		{Name: "PT-101", Meta: core.Meta{Unit: "bar", Max: 10, Hi: 8, Desc: "反应釜压力"}},
	})

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "DictUI"})

	seed := `s.Tags.GetOrCreate("PT-101", core.Meta{Unit: "bar", Max: 10, Hi: 8, Desc: "反应釜压力"})`
	if !strings.Contains(code, seed) {
		t.Fatalf("generated code missing the seeded declaration:\n  %s\n----\n%s", seed, code)
	}
	bind := strings.Index(code, "scada.BindScreen(")
	if bind < 0 {
		t.Fatalf("declaring a tag did not make the design services-backed\n----\n%s", code)
	}
	if strings.Index(code, seed) > bind {
		t.Errorf("declared tag is seeded after scada.BindScreen; Meta would be lost\n----\n%s", code)
	}

	vetGeneratedCode(t, code)
}

// TestGenerateCodeSeedsBeforeSetTagName: a tag both declared and bound to a
// widget must be seeded once, from the declaration, and still be handed to the
// widget — the two paths write different things (registry meta vs widget seam)
// and neither may replace the other.
func TestGenerateCodeSeedsBoundTagOnce(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Plant")
	scene.SetSize(240, 180)
	addFakeWithTag(t, scene, "gui.Tank", "tank1", "PT-101")
	scene.SetTagDict([]TagDecl{{Name: "PT-101", Meta: core.Meta{Unit: "bar", Max: 10}}})

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "PlantUI"})

	if n := strings.Count(code, `s.Tags.GetOrCreate("PT-101"`); n != 1 {
		t.Errorf("seeded PT-101 %d times, want exactly 1\n----\n%s", n, code)
	}
	if !strings.Contains(code, `ui.Tank1.SetTagName("PT-101")`) {
		t.Errorf("bound widget lost its tag\n----\n%s", code)
	}
}

// TestGenerateCodeEmptyDictSeedsNothing: a design that declares nothing keeps the
// output it had before the dictionary existed — no seeding block at all.
func TestGenerateCodeEmptyDictSeedsNothing(t *testing.T) {
	scene := NewGedScene()
	scene.SetFormTitle("Plant")
	scene.SetSize(240, 180)
	addFakeWithTag(t, scene, "gui.Tank", "tank1", "PT-101")

	code := scene.GenerateCode(CodeGenOptions{PackageName: "main", TypeName: "PlantUI"})

	if strings.Contains(code, "s.Tags.GetOrCreate(") {
		t.Errorf("undeclared design emitted a seeding call\n----\n%s", code)
	}
}
