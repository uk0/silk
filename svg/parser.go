package svg

import (
	"encoding/xml"
	"errors"
	"fmt"
	"image/color"
	"io"
	"strconv"
	"strings"

	"silk/paint"
)

// Parse decodes SVG bytes into a Doc tree. Returns an error on malformed
// XML or any structural problem with the root <svg> element. Unknown
// child elements are silently skipped — partial-support degradation
// matches what every browser does with unfamiliar SVG features.
//
// Coordinate normalisation: every path "d" attribute is decoded into
// absolute commands at parse time; the renderer doesn't need to
// maintain a "current point" outside of the path it's rendering.
func Parse(data []byte) (*Doc, error) {
	dec := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return nil, errors.New("svg: no <svg> root element")
			}
			return nil, err
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		if start.Name.Local != "svg" {
			return nil, fmt.Errorf("svg: root element is %q, want svg", start.Name.Local)
		}
		return parseRoot(dec, start)
	}
}

// ParseString is a convenience for the common case of parsing a Go
// string literal SVG (e.g. an embedded icon resource).
func ParseString(s string) (*Doc, error) { return Parse([]byte(s)) }

func parseRoot(dec *xml.Decoder, start xml.StartElement) (*Doc, error) {
	doc := &Doc{}
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "width":
			doc.Width = parseLength(a.Value)
		case "height":
			doc.Height = parseLength(a.Value)
		case "viewBox":
			doc.ViewBox = parseViewBox(a.Value)
		}
	}
	children, err := parseChildren(dec)
	if err != nil {
		return nil, err
	}
	doc.Children = children
	return doc, nil
}

// parseChildren reads tokens up to the matching EndElement and returns
// every successfully-parsed shape. Unrecognised elements are skipped
// via skipElement, so the parser tolerates extension namespaces (Inkscape,
// Sodipodi) without complaint.
func parseChildren(dec *xml.Decoder) ([]Shape, error) {
	var out []Shape
	for {
		tok, err := dec.Token()
		if err != nil {
			if err == io.EOF {
				return out, nil
			}
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			s, err := parseElement(dec, t)
			if err != nil {
				return nil, err
			}
			if s != nil {
				out = append(out, s)
			}
		case xml.EndElement:
			return out, nil
		}
	}
}

// parseElement dispatches by tag name.
func parseElement(dec *xml.Decoder, start xml.StartElement) (Shape, error) {
	switch start.Name.Local {
	case "rect":
		return parseRect(start), skip(dec, start)
	case "circle":
		return parseCircle(start), skip(dec, start)
	case "ellipse":
		return parseEllipse(start), skip(dec, start)
	case "line":
		return parseLine(start), skip(dec, start)
	case "polygon":
		return parsePolygon(start), skip(dec, start)
	case "polyline":
		return parsePolyline(start), skip(dec, start)
	case "path":
		s, err := parsePath(start)
		if err != nil {
			return nil, err
		}
		return s, skip(dec, start)
	case "g":
		g := &Group{}
		applyCommonAttrs(&g.Common, start.Attr)
		kids, err := parseChildren(dec)
		if err != nil {
			return nil, err
		}
		g.Children = kids
		return g, nil
	default:
		// Skip unknown elements — defs, title, metadata, etc.
		return nil, skip(dec, start)
	}
}

// skip walks tokens until the matching EndElement of start. Used after
// shapes that don't accept nested children (rect / circle / etc).
func skip(dec *xml.Decoder, start xml.StartElement) error {
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return nil
}

// applyCommonAttrs reads style + transform from attr into c.
func applyCommonAttrs(c *Common, attrs []xml.Attr) {
	// Inline-style attributes carry into Common.Style; same for
	// presentation attributes. We explicitly set each pointer field
	// when seen so a missing attribute reads back as nil ("inherit").
	for _, a := range attrs {
		switch a.Name.Local {
		case "fill":
			col := parseColor(a.Value)
			c.Style.Fill = &col
		case "stroke":
			col := parseColor(a.Value)
			c.Style.Stroke = &col
		case "stroke-width":
			v := parseLength(a.Value)
			c.Style.StrokeWidth = &v
		case "opacity":
			v := parseLength(a.Value)
			c.Style.Opacity = &v
		case "fill-opacity":
			v := parseLength(a.Value)
			c.Style.FillOpacity = &v
		case "stroke-opacity":
			v := parseLength(a.Value)
			c.Style.StrokeOpacity = &v
		case "fill-rule":
			switch a.Value {
			case "nonzero":
				c.Style.FillRule = FillRuleNonzero
			case "evenodd":
				c.Style.FillRule = FillRuleEvenOdd
			}
		case "style":
			parseInlineStyle(a.Value, &c.Style)
		case "transform":
			c.Transform = parseTransform(a.Value)
		}
	}
}

// parseInlineStyle splits a style="prop:val; prop:val" attribute into
// individual property assignments and routes them through the same
// setters as presentation attributes.
func parseInlineStyle(s string, st *Style) {
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		colon := strings.IndexByte(part, ':')
		if colon <= 0 {
			continue
		}
		prop := strings.TrimSpace(part[:colon])
		val := strings.TrimSpace(part[colon+1:])
		switch prop {
		case "fill":
			c := parseColor(val)
			st.Fill = &c
		case "stroke":
			c := parseColor(val)
			st.Stroke = &c
		case "stroke-width":
			v := parseLength(val)
			st.StrokeWidth = &v
		case "opacity":
			v := parseLength(val)
			st.Opacity = &v
		case "fill-opacity":
			v := parseLength(val)
			st.FillOpacity = &v
		case "stroke-opacity":
			v := parseLength(val)
			st.StrokeOpacity = &v
		case "fill-rule":
			switch val {
			case "nonzero":
				st.FillRule = FillRuleNonzero
			case "evenodd":
				st.FillRule = FillRuleEvenOdd
			}
		}
	}
}

// --- Shape parsers ---------------------------------------------------

func parseRect(start xml.StartElement) *Rect {
	r := &Rect{}
	applyCommonAttrs(&r.Common, start.Attr)
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x":
			r.X = parseLength(a.Value)
		case "y":
			r.Y = parseLength(a.Value)
		case "width":
			r.W = parseLength(a.Value)
		case "height":
			r.H = parseLength(a.Value)
		case "rx":
			r.Rx = parseLength(a.Value)
		case "ry":
			r.Ry = parseLength(a.Value)
		}
	}
	// rx-only or ry-only: SVG spec says use the other one as fallback.
	if r.Rx > 0 && r.Ry == 0 {
		r.Ry = r.Rx
	}
	if r.Ry > 0 && r.Rx == 0 {
		r.Rx = r.Ry
	}
	return r
}

func parseCircle(start xml.StartElement) *Circle {
	c := &Circle{}
	applyCommonAttrs(&c.Common, start.Attr)
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "cx":
			c.Cx = parseLength(a.Value)
		case "cy":
			c.Cy = parseLength(a.Value)
		case "r":
			c.R = parseLength(a.Value)
		}
	}
	return c
}

func parseEllipse(start xml.StartElement) *Ellipse {
	e := &Ellipse{}
	applyCommonAttrs(&e.Common, start.Attr)
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "cx":
			e.Cx = parseLength(a.Value)
		case "cy":
			e.Cy = parseLength(a.Value)
		case "rx":
			e.Rx = parseLength(a.Value)
		case "ry":
			e.Ry = parseLength(a.Value)
		}
	}
	return e
}

func parseLine(start xml.StartElement) *Line {
	l := &Line{}
	applyCommonAttrs(&l.Common, start.Attr)
	for _, a := range start.Attr {
		switch a.Name.Local {
		case "x1":
			l.X1 = parseLength(a.Value)
		case "y1":
			l.Y1 = parseLength(a.Value)
		case "x2":
			l.X2 = parseLength(a.Value)
		case "y2":
			l.Y2 = parseLength(a.Value)
		}
	}
	return l
}

func parsePolygon(start xml.StartElement) *Polygon {
	p := &Polygon{}
	applyCommonAttrs(&p.Common, start.Attr)
	for _, a := range start.Attr {
		if a.Name.Local == "points" {
			p.Points = parsePoints(a.Value)
		}
	}
	return p
}

func parsePolyline(start xml.StartElement) *Polyline {
	p := &Polyline{}
	applyCommonAttrs(&p.Common, start.Attr)
	for _, a := range start.Attr {
		if a.Name.Local == "points" {
			p.Points = parsePoints(a.Value)
		}
	}
	return p
}

func parsePath(start xml.StartElement) (*Path, error) {
	p := &Path{}
	applyCommonAttrs(&p.Common, start.Attr)
	for _, a := range start.Attr {
		if a.Name.Local == "d" {
			cmds, err := parsePathData(a.Value)
			if err != nil {
				return nil, fmt.Errorf("svg path data: %w", err)
			}
			p.Commands = cmds
		}
	}
	return p, nil
}

// --- Primitive value parsers ----------------------------------------

func parseLength(s string) float64 {
	s = strings.TrimSpace(s)
	// Strip common units; SVG icons rarely depend on exact unit
	// resolution beyond unitless values.
	s = strings.TrimSuffix(s, "px")
	s = strings.TrimSuffix(s, "pt")
	s = strings.TrimSuffix(s, "%")
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func parseViewBox(s string) ViewBox {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	if len(parts) != 4 {
		return ViewBox{}
	}
	return ViewBox{
		X: parseLength(parts[0]),
		Y: parseLength(parts[1]),
		W: parseLength(parts[2]),
		H: parseLength(parts[3]),
	}
}

// parsePoints turns "10,20 30,40 50,60" into a Point slice. Handles
// comma- and whitespace-separated forms interchangeably (per spec).
func parsePoints(s string) []Point {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	var out []Point
	for i := 0; i+1 < len(parts); i += 2 {
		x := parseLength(parts[i])
		y := parseLength(parts[i+1])
		out = append(out, Point{X: x, Y: y})
	}
	return out
}

// parseColor handles the subset of CSS colour syntax SVG icons use:
// "#rgb", "#rrggbb", "rgb(r,g,b)", "rgba(r,g,b,a)", "none", and the
// 16 basic CSS keywords (red, blue, ...). Unknown forms fall back to
// transparent black so a malformed icon at least renders as nothing.
func parseColor(s string) Color {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "none" || s == "" {
		return Color{None: true}
	}
	if v, ok := namedColors[s]; ok {
		return Color{Val: v}
	}
	if strings.HasPrefix(s, "#") {
		return Color{Val: parseHexColor(s)}
	}
	if strings.HasPrefix(s, "rgb(") || strings.HasPrefix(s, "rgba(") {
		return Color{Val: parseRGBColor(s)}
	}
	return Color{}
}

func parseHexColor(s string) paint.Color {
	s = strings.TrimPrefix(s, "#")
	switch len(s) {
	case 3:
		// Short form: #abc → #aabbcc
		r := hexNibble(s[0])
		g := hexNibble(s[1])
		b := hexNibble(s[2])
		return paint.Color{R: r*16 + r, G: g*16 + g, B: b*16 + b, A: 255}
	case 6:
		r, _ := strconv.ParseUint(s[0:2], 16, 8)
		g, _ := strconv.ParseUint(s[2:4], 16, 8)
		b, _ := strconv.ParseUint(s[4:6], 16, 8)
		return paint.Color{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
	case 8:
		r, _ := strconv.ParseUint(s[0:2], 16, 8)
		g, _ := strconv.ParseUint(s[2:4], 16, 8)
		b, _ := strconv.ParseUint(s[4:6], 16, 8)
		a, _ := strconv.ParseUint(s[6:8], 16, 8)
		return paint.Color{R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(a)}
	}
	return paint.Color{}
}

func hexNibble(c byte) uint8 {
	switch {
	case c >= '0' && c <= '9':
		return uint8(c - '0')
	case c >= 'a' && c <= 'f':
		return uint8(c-'a') + 10
	case c >= 'A' && c <= 'F':
		return uint8(c-'A') + 10
	}
	return 0
}

func parseRGBColor(s string) paint.Color {
	open := strings.IndexByte(s, '(')
	close := strings.IndexByte(s, ')')
	if open < 0 || close < 0 || close <= open+1 {
		return paint.Color{}
	}
	parts := strings.Split(s[open+1:close], ",")
	if len(parts) < 3 {
		return paint.Color{}
	}
	r := uint8(parseLength(parts[0]))
	g := uint8(parseLength(parts[1]))
	b := uint8(parseLength(parts[2]))
	a := uint8(255)
	if len(parts) >= 4 {
		// rgba's alpha is a 0..1 float.
		af := parseLength(parts[3])
		if af < 0 {
			af = 0
		} else if af > 1 {
			af = 1
		}
		a = uint8(af * 255)
	}
	_ = color.RGBA{} // import keeps tooling happy if not otherwise used
	return paint.Color{R: r, G: g, B: b, A: a}
}

// namedColors is the CSS named colour map for the 16 basic colours
// plus a few extras commonly used in icon art. Lower-cased keys.
var namedColors = map[string]paint.Color{
	"black":   {R: 0, G: 0, B: 0, A: 255},
	"white":   {R: 255, G: 255, B: 255, A: 255},
	"red":     {R: 255, G: 0, B: 0, A: 255},
	"green":   {R: 0, G: 128, B: 0, A: 255},
	"blue":    {R: 0, G: 0, B: 255, A: 255},
	"yellow":  {R: 255, G: 255, B: 0, A: 255},
	"cyan":    {R: 0, G: 255, B: 255, A: 255},
	"magenta": {R: 255, G: 0, B: 255, A: 255},
	"gray":    {R: 128, G: 128, B: 128, A: 255},
	"grey":    {R: 128, G: 128, B: 128, A: 255},
	"silver":  {R: 192, G: 192, B: 192, A: 255},
	"maroon":  {R: 128, G: 0, B: 0, A: 255},
	"olive":   {R: 128, G: 128, B: 0, A: 255},
	"lime":    {R: 0, G: 255, B: 0, A: 255},
	"aqua":    {R: 0, G: 255, B: 255, A: 255},
	"teal":    {R: 0, G: 128, B: 128, A: 255},
	"navy":    {R: 0, G: 0, B: 128, A: 255},
	"fuchsia": {R: 255, G: 0, B: 255, A: 255},
	"purple":  {R: 128, G: 0, B: 128, A: 255},
	"orange":  {R: 255, G: 165, B: 0, A: 255},
}

// --- Transform parser -----------------------------------------------

// parseTransform decodes a SVG transform attribute string into a
// Transform with one TransformOp per function call. We honour the
// canonical four (translate / scale / rotate / matrix); skewX / skewY
// are intentionally omitted — uncommon in icon art and adding them
// would more than double the parser size.
func parseTransform(s string) Transform {
	var t Transform
	i := 0
	for i < len(s) {
		// Skip leading whitespace / commas.
		for i < len(s) && (s[i] == ' ' || s[i] == ',' || s[i] == '\t' || s[i] == '\n') {
			i++
		}
		if i >= len(s) {
			break
		}
		// Read function name up to '('.
		start := i
		for i < len(s) && s[i] != '(' {
			i++
		}
		if i >= len(s) {
			break
		}
		name := strings.TrimSpace(s[start:i])
		i++ // skip '('
		// Collect args until ')'.
		argStart := i
		for i < len(s) && s[i] != ')' {
			i++
		}
		args := parseTransformArgs(s[argStart:i])
		if i < len(s) {
			i++ // skip ')'
		}
		op := buildTransformOp(name, args)
		if op.Kind != 0 {
			t.Ops = append(t.Ops, op)
		}
	}
	return t
}

func parseTransformArgs(s string) []float64 {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	})
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		out = append(out, parseLength(p))
	}
	return out
}

func buildTransformOp(name string, args []float64) TransformOp {
	switch name {
	case "translate":
		op := TransformOp{Kind: TransformTranslate}
		if len(args) >= 1 {
			op.X = args[0]
		}
		if len(args) >= 2 {
			op.Y = args[1]
		}
		return op
	case "scale":
		op := TransformOp{Kind: TransformScale}
		if len(args) >= 1 {
			op.X = args[0]
			op.Y = args[0]
		}
		if len(args) >= 2 {
			op.Y = args[1]
		}
		return op
	case "rotate":
		op := TransformOp{Kind: TransformRotate}
		if len(args) >= 1 {
			op.X = args[0]
		}
		if len(args) >= 3 {
			op.A = args[1]
			op.B = args[2]
			op.Has = true
		}
		return op
	case "matrix":
		if len(args) < 6 {
			return TransformOp{}
		}
		return TransformOp{
			Kind: TransformMatrix,
			A:    args[0], B: args[1],
			C: args[2], D: args[3],
			E: args[4], F: args[5],
		}
	}
	return TransformOp{}
}
