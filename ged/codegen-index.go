package ged

import (
	"fmt"
	"strings"
)

// GeneratedCode is one pass of GenerateCodeIndexed: the source, where in it
// each designed widget is built, and whether the source is a faithful
// rendering of the design.
type GeneratedCode struct {
	Code string

	// Lines maps a designed widget to the 0-based line of the statement that
	// constructs it. A widget the generator emitted nothing for is absent.
	Lines map[*FakeWidget]int

	// Err names what the generator could not render. Code is still filled in —
	// it is the degraded output — but a view showing it as the design's code
	// would be lying, so viewers show Err in its place.
	Err error
}

// Line returns the 0-based line fake is constructed on, and whether it is in
// the map at all.
func (g GeneratedCode) Line(fake *FakeWidget) (int, bool) {
	line, ok := g.Lines[fake]
	return line, ok
}

// constructorAnchor is the leading text of the first statement GenerateCode
// emits for a field. Field names are made unique by the collect walk, and the
// " = " terminator keeps "ui.Btn" from matching the "ui.Btn_2" declared after
// it, so one anchor names exactly one statement.
func constructorAnchor(fieldName string) string {
	return "ui." + fieldName + " = "
}

// indexAnchors resolves each anchor to its 0-based line in src, or -1 when it
// is not there. The anchors must be given in the order they were emitted: the
// scan only ever moves forward, so a widget's field name reappearing in a
// handler body further down cannot steal a later widget's line.
func indexAnchors(src string, anchors []string) []int {
	at := make([]int, len(anchors))
	for i := range at {
		at[i] = -1
	}
	next := 0
	for lineNo, line := range strings.Split(src, "\n") {
		if next >= len(anchors) {
			break
		}
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), anchors[next]) {
			at[next] = lineNo
			next++
		}
	}
	return at
}

// unmappedFactoryError describes the widgets codegen has no mapping for, by
// name, or returns nil when there are none. The message is what the 生成代码
// view shows in place of the code.
func unmappedFactoryError(unmapped []*FakeWidget) error {
	if len(unmapped) == 0 {
		return nil
	}
	names := make([]string, 0, len(unmapped))
	for _, fake := range unmapped {
		name := fake.WidgetName()
		if name == "" {
			name = "未命名"
		}
		names = append(names, fmt.Sprintf("%s (%s)", name, fake.WidgetFactoryName()))
	}
	return fmt.Errorf("无法生成代码: 以下控件没有代码生成映射: %s", strings.Join(names, ", "))
}
