package ged

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/decl"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/internal/silkgen"
)

func TestIntegerRangeDesignSaveReloadPreview(t *testing.T) {
	for _, vals := range [][3]int{{-100, 100, -42}, {200, 300, 250}, {-300, -200, -250}, {math.MinInt, math.MaxInt, math.MaxInt - 1}, {math.MinInt, math.MaxInt, math.MinInt + 1}} {
		t.Run(fmt.Sprint(vals), func(t *testing.T) {
			fw, err := NewFakeWidgetFromFactory("gui.SpinBox")
			if err != nil {
				t.Fatal(err)
			}
			original := fw.Widget().(*gui.SpinBox)
			original.SetRange(vals[0], vals[1])
			original.SetValue(vals[2])
			original.SetStep(7)
			original.SetSuffix(" items")
			for round := 0; round < 3; round++ {
				doc, err := core.LoadTDocStr(fw.SaveDesign().String())
				if err != nil {
					t.Fatal(err)
				}
				fw, err = NewFakeWidgetFromFactory("gui.SpinBox")
				if err != nil {
					t.Fatal(err)
				}
				if err = fw.LoadDesign(doc); err != nil {
					t.Fatal(err)
				}
				for phase, w := range map[string]gui.IWidget{"designer": fw.Widget(), "preview": fw.Generate()} {
					s := w.(*gui.SpinBox)
					if got := ([3]int{s.Min(), s.Max(), s.Value()}); got != vals {
						t.Fatalf("round%d %s: got%v want%v", round, phase, got, vals)
					}
					if s.Step() != 7 || s.Suffix() != " items" {
						t.Fatal("lost unrelated properties")
					}
				}
			}
		})
	}
}

type readOnlyIntegerRange struct{ *gui.SpinBox }

func (s readOnlyIntegerRange) EnumProperties(list core.IPropertyList) {
	list.AddProperty("最小值", s.Min, nil)
	list.AddProperty("最大值", s.Max, nil)
	list.AddProperty("值", s.Value, s.SetValue)
}
func TestIntegerRangeRestoreRespectsWritableProperties(t *testing.T) {
	s := gui.NewSpinBox()
	values := map[string]string{"最小值": "-100", "最大值": "200", "值": "42", "unknown": "bad"}
	copy := map[string]string{}
	for k, v := range values {
		copy[k] = v
	}
	applyWidgetProperties(readOnlyIntegerRange{s}, values)
	if s.Min() != 0 || s.Max() != 99 || s.Value() != 42 {
		t.Fatal("read-only ranges modified")
	}
	if !reflect.DeepEqual(values, copy) {
		t.Fatal("caller properties mutated")
	}
}

// Compile and execute both public generation paths against the current SDK.
// This catches valid Go that nevertheless clamps values during construction.
func TestIntegerRangeGeneratedSDKRuntime(t *testing.T) {
	if testing.Short() {
		t.Skip("external SDK consumer compilation")
	}
	scene := NewGedScene()
	scene.SetSize(180, 100)
	values := [][3]int{{-100, 100, -42}, {200, 300, 250}, {-300, -200, -250}, {math.MinInt, math.MaxInt, math.MaxInt - 1}, {math.MinInt, math.MaxInt, math.MinInt + 1}}
	var checks strings.Builder
	for i, vals := range values {
		name := fmt.Sprintf("Counter%d", i)
		addConfiguredFake(t, scene, "gui.SpinBox", name, func(w interface{}) { s := w.(*gui.SpinBox); s.SetRange(vals[0], vals[1]); s.SetValue(vals[2]) })
		fmt.Fprintf(&checks, "check(t, ui.%s, %d, %d, %d); check(t, index[%q].(*gui.SpinBox), %d, %d, %d)\n", name, vals[0], vals[1], vals[2], name, vals[0], vals[1], vals[2])
	}
	loaded := NewGedScene()
	if err := loaded.LoadDesign(scene.SaveDesign()); err != nil {
		t.Fatal(err)
	}
	concrete := loaded.GenerateCodeIndexed(CodeGenOptions{PackageName: "rangefixture", TypeName: "RangeUI"})
	if concrete.Err != nil {
		t.Fatal(concrete.Err)
	}
	tree, err := decl.FromTDoc(loaded.SaveDesign())
	if err != nil {
		t.Fatal(err)
	}
	input := "ranges.silkui"
	declarative, err := silkgen.GenerateSource(input, "rangefixture", "", tree)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":      fmt.Sprintf("module rangefixture\ngo 1.26.0\nrequire github.com/uk0/silk v0.0.0\nreplace github.com/uk0/silk => %s\n", strconv.Quote(findModuleRoot(t))),
		"concrete.go": concrete.Code, "declarative.go": string(declarative),
		"runtime_test.go": fmt.Sprintf(`package rangefixture
import("testing";"github.com/uk0/silk/gui")
func check(t *testing.T,s *gui.SpinBox,min,max,value int){t.Helper();if s.Min()!=min||s.Max()!=max||s.Value()!=value{t.Fatalf("got [%%d %%d %%d], want [%%d %%d %%d]",s.Min(),s.Max(),s.Value(),min,max,value)}}
func TestIntegerGeneratedRuntime(t *testing.T){ui:=NewRangeUI();_,index,err:=%s().BuildWithIndex();if err!=nil{t.Fatal(err)};%s}
`, silkgen.DefaultBuildName(input), checks.String()),
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	// A cold Windows race build can consume almost the entire old 120-second
	// combined deadline. Bound compilation independently so a slow compiler is
	// not reported as a failed SDK runtime assertion. Keep execution much tighter
	// and let the test binary print its own timeout stack before the outer kill.
	binary := filepath.Join(dir, "rangefixture.test")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer buildCancel()
	childEnv := append(os.Environ(), "GOMAXPROCS=2", "SILK_DATA_DIR="+filepath.Join(dir, "data"))
	cmd := exec.CommandContext(buildCtx, "go", "test", "-c", "-race", "-mod=mod", "-p", "1", "-o", binary, ".")
	cmd.Dir = dir
	cmd.Env = childEnv
	cmd.WaitDelay = 5 * time.Second
	started := time.Now()
	out, err := cmd.CombinedOutput()
	buildErr := buildCtx.Err()
	buildCancel()
	if err != nil {
		t.Fatalf("generated SDK build after %s: %v (context: %v)\n%s", time.Since(started), err, buildErr, out)
	}
	t.Logf("generated SDK race build: %s", time.Since(started))
	runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer runCancel()
	cmd = exec.CommandContext(runCtx, binary, "-test.paniconexit0", "-test.timeout=25s", "-test.count=1", "-test.run=^TestIntegerGeneratedRuntime$", "-test.v")
	cmd.Dir = dir
	cmd.Env = childEnv
	cmd.WaitDelay = 5 * time.Second
	started = time.Now()
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generated SDK execution after %s: %v (context: %v)\n%s", time.Since(started), err, runCtx.Err(), out)
	}
	if !strings.Contains(string(out), "--- PASS: TestIntegerGeneratedRuntime") {
		t.Fatalf("generated SDK binary did not run the required assertion:\n%s", out)
	}
	t.Logf("both generated constructors verified:\n%s", out)
}
