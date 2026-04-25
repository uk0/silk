package wmap

import (
	"silk/graph"
	//	"silk/gui"
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
