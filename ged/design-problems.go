package ged

import (
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/graph"
)

// What the designer knows is wrong with a design, in the form the 问题 pane
// shows it. Two things go missing without anyone being told:
//
//   - a widget whose factory this build does not know is dropped by
//     GedScene.Generate and by GenerateCode, taking its children with it, so
//     the preview, the exported file and the run are all quietly incomplete;
//   - a widget whose factory was unknown at load time never entered the scene
//     at all, and saving over the file makes that permanent.
//
// Both used to end at a core.Warn nobody has open.

// unbuildableWidgets lists every widget in the scene FakeWidget.Generate
// cannot construct, in scene order.
//
// Generate builds each widget through NewFakeWidgetFromFactory and returns nil
// when that fails, and both callers — GedScene.Generate and the nested-child
// loop — skip a nil silently. So an unbuildable widget does not fail the
// build, it disappears from it, together with everything nested inside it;
// that is why a blocker's children are not walked here either.
//
// The condition tested is factory registration, the one that varies between a
// document and the program reading it. A FakeWidget's factory name is always
// derived from a live widget instance (SetWidget -> core.FactoryNameOf), so the
// other half of Generate's test — that the factory makes a gui.IWidget — cannot
// fail for a name that resolves at all, and is not worth constructing every
// widget in the design a second time to ask.
func unbuildableWidgets(scene *GedScene) []*FakeWidget {
	if scene == nil {
		return nil
	}
	var out []*FakeWidget
	var walk func(items []graph.IItem)
	walk = func(items []graph.IItem) {
		for _, item := range items {
			fake, ok := item.(*FakeWidget)
			if !ok {
				continue
			}
			if core.FindFactory(fake.WidgetFactoryName()) == nil {
				out = append(out, fake)
				continue
			}
			if fake.HasChildren() {
				walk(fake.Children())
			}
		}
	}
	walk(scene.Children())
	return out
}

// DesignProblems reports what a preview, an export or a run would lose from
// this design. Each row carries the widget at fault so the 问题 pane can select
// it on the canvas — the widget is in the scene, it just cannot be built.
func DesignProblems(scene *GedScene) []Problem {
	var out []Problem
	for _, fake := range unbuildableWidgets(scene) {
		out = append(out, Problem{
			File:     previewBlockerLabel(fake.WidgetName(), fake.WidgetFactoryName()),
			Severity: SeverityError,
			Message:  "无法生成该控件: 未注册的类型 " + fake.WidgetFactoryName(),
			Item:     fake,
		})
	}
	return out
}

// LoadProblems turns the factory names a load had to skip into rows. They
// carry no Item: those widgets were never built, so there is nothing on the
// canvas to select. The message names the consequence rather than the event,
// because saving the partially-loaded design is what makes the loss permanent.
func LoadProblems(missing []string) []Problem {
	var out []Problem
	for _, factoryName := range missing {
		if factoryName == "" {
			factoryName = "(空类型名)"
		}
		out = append(out, Problem{
			File:     factoryName,
			Severity: SeverityWarning,
			Message:  "载入时跳过了未知控件类型; 保存后该控件将从设计中永久丢失",
		})
	}
	return out
}
