// A timer-free 3D industrial scene: orbit, pick, load OBJ and publish tag data.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"image/color"
	"image/png"
	"os"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/scene3d"
)

type demo struct {
	gui.Form
	view          *gui.SceneView
	title, status *gui.Label
	buttons       []*gui.Button
	tag           *core.Tag
	loaded        int
	ctx           context.Context
	cancel        context.CancelFunc
}

func newDemo(model string) *demo {
	f := new(demo)
	f.Form.Init(f)
	f.SetTitle("Silk - Industrial 3D Scene")
	f.ctx, f.cancel = context.WithCancel(context.Background())
	f.view = gui.NewSceneView()
	f.view.SetParent(f)
	f.view.SetModelSource(model)
	f.title = gui.NewLabel("3D Scene · CPU reference | Drag: orbit · Wheel: zoom · Click: select")
	f.title.SetParent(f)
	f.status = gui.NewLabel("")
	f.status.SetParent(f)
	s := scene3d.NewScene()
	mesh := scene3d.Box()
	tankMesh, err := scene3d.Cylinder(1, 2, 40)
	if err != nil {
		panic(err)
	}
	for _, n := range []scene3d.Node{{ID: "floor", Mesh: mesh, Color: color.RGBA{100, 115, 140, 255}, Transform: scene3d.Translation(scene3d.Vec3{Y: -1.2}).Mul(scene3d.Scaling(scene3d.Vec3{4, .1, 3}))}, {ID: "tank", Mesh: tankMesh, Color: color.RGBA{40, 190, 170, 255}, Transform: scene3d.Translation(scene3d.Vec3{X: -1}).Mul(scene3d.Scaling(scene3d.Vec3{.5, 1, .5}))}, {ID: "pump", Mesh: mesh, Color: color.RGBA{245, 160, 65, 255}, Transform: scene3d.Translation(scene3d.Vec3{X: 1, Y: -.7}).Mul(scene3d.Scaling(scene3d.Vec3{.8, .4, .5}))}} {
		if err := s.Add(n); err != nil {
			panic(err)
		}
	}
	if err := f.view.SetScene(s); err != nil {
		panic(err)
	}
	f.tag = core.NewTagDB().GetOrCreate("tank.level", core.Meta{})
	f.tag.SetValue(0.0)
	if _, err := f.view.BindNodePosition(f.tag, "tank", 1, .001, 0); err != nil {
		panic(err)
	}
	f.view.SigSelected(func(string) { f.snapshot() })
	f.view.SigModelLoaded(func() {
		f.loaded++
		if _, err := f.view.BindNodePosition(f.tag, "model", 1, .001, 0); err != nil {
			panic(err)
		}
		gui.Post(f.snapshot)
	})
	f.view.SigError(func(string) { f.snapshot() })
	add := func(text string, fn func()) {
		b := gui.NewButton1(text, nil)
		b.SetParent(f)
		b.Action().BindFunc0(fn)
		f.buttons = append(f.buttons, b)
	}
	add("Publish 1000 values", func() {
		tag, ctx := f.tag, f.ctx
		go func() {
			for i := 0; i < 1000; i++ {
				if ctx.Err() != nil {
					return
				}
				tag.SetValue(float64(i))
			}
			_ = gui.PostContext(ctx, f.snapshot)
		}()
	})
	add("Load OBJ", func() {
		if err := f.view.LoadModel(f.ctx); err != nil {
			fmt.Println("LOAD_ERROR", err)
		}
		f.snapshot()
	})
	add("Reset camera", func() { f.view.SetYaw(.65); f.view.SetPitch(.35); f.view.SetDistance(6); f.snapshot() })
	add("Snapshot", f.snapshot)
	f.snapshot()
	return f
}
func (f *demo) snapshot() {
	id := "tank"
	n, ok := f.view.Scene().Node(id)
	if !ok {
		id = "model"
		n, _ = f.view.Scene().Node(id)
	}
	err := ""
	if f.view.LastError() != nil {
		err = f.view.LastError().Error()
	}
	state := map[string]any{"node": id, "y": n.Transform[13], "selected": f.view.SelectedNode(), "loaded": f.loaded, "busy": f.view.IsLoading(), "renders": f.view.RenderCount(), "on_ui": gui.IsUIThread(), "error": err, "yaw": f.view.Yaw(), "pitch": f.view.Pitch(), "distance": f.view.Distance(), "warnings": f.view.ModelWarnings()}
	b, _ := json.Marshal(state)
	fmt.Println("SCENE_STATE", string(b))
	text := fmt.Sprintf("Node: %s  |  Y: %.3f  |  Selected: %s  |  Loads: %d  |  Renders: %d", id, n.Transform[13], f.view.SelectedNode(), f.loaded, f.view.RenderCount())
	if f.view.IsLoading() {
		text += "  |  Loading…"
	} else if err != "" {
		text = "Error: " + err
	} else if warnings := f.view.ModelWarnings(); len(warnings) > 0 {
		text += "  |  " + warnings[0]
	}
	f.status.SetText(text)
}
func (f *demo) OnResize() {
	w, h := f.Size()
	f.title.SetBounds(12, 8, max(0, w-24), 26)
	for i, b := range f.buttons {
		bw := max(0, (w-24)/4)
		b.SetBounds(12+float64(i)*bw, 40, max(0, bw-6), 32)
	}
	f.view.SetBounds(12, 84, max(0, w-24), max(0, h-122))
	f.status.SetBounds(12, max(0, h-30), max(0, w-24), 24)
}
func main() {
	model := flag.String("model", "examples/scene3d/pump.obj", "explicit OBJ path")
	render := flag.String("render", "", "write reference PNG and exit without a window")
	flag.Parse()
	defer core.Close()
	f := newDemo(*model)
	defer func() { f.cancel(); f.view.Close() }()
	if *render != "" {
		snap, err := f.view.Scene().Snapshot()
		if err != nil {
			panic(err)
		}
		var r scene3d.Renderer
		frame, err := r.Render(snap, scene3d.DefaultCamera(), 960, 640, color.RGBA{20, 28, 41, 255})
		if err != nil {
			panic(err)
		}
		out, err := os.Create(*render)
		if err != nil {
			panic(err)
		}
		err = png.Encode(out, frame.Image)
		closeErr := out.Close()
		if err != nil {
			panic(err)
		}
		if closeErr != nil {
			panic(closeErr)
		}
		fmt.Println("RENDER_STATS", frame.Stats)
		return
	}
	if err := f.TryAttachWindow(gui.WtForm); err != nil {
		panic(err)
	}
	f.Window().SetSize(980, 680)
	f.Window().MoveToCenter()
	f.Show()
	core.EventLoop()
}
