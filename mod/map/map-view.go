package wmap

import (
	"github.com/uk0/silk/graph"
	//	"github.com/uk0/silk/gui"
)

func NewMapView() *MapView {
	p := new(MapView)
	p.Init(p)
	p.SetScene(NewMapScene())
	return p
}

type MapView struct {
	graph.GraphView
}
