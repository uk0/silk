package wmap

import (
	"github.com/uk0/silk/graph"
	//	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/paint"
)

func NewMapScene() *MapScene {
	p := new(MapScene)
	p.Init(p)
	p.SetSize(200, 200)
	p.SetPropertyConfigName("map")
	//p.SetSelectable(true)
	return p
}

type MapScene struct {
	graph.SceneItem
}

func (this *MapScene) DrawSelf(g paint.Painter) {
	//g.Rectangle(this.X(), this.Y(), this.Width(), this.Height())
	//g.SetSourceColor(paint.Color{255, 255, 255, 255})
	//g.Fill()
}

func (this *MapScene) Icon() paint.Icon {
	return paint.LoadIcon("map")
}

func (this *MapScene) Title() string {
	return "平面图"
}
