// Package svg is Silk's SVG rendering layer — the equivalent of Qt's
// QSvgRenderer. It parses SVG XML into a small AST and renders the AST
// through silk/paint.Painter, so the same drawing pipeline used by
// every Silk widget also handles SVG icons.
//
// Scope: the parser handles the dominant subset of SVG used in icon
// art — rect / circle / ellipse / line / polygon / polyline / path
// shapes, group nesting, basic transforms (translate / scale / rotate /
// matrix), and the Style attributes fill / stroke / stroke-width /
// opacity / fill-rule. Out of scope:
//
//   - text (<text>, <tspan>) — Silk widgets typically render text
//     through paint.Painter directly anyway
//   - filters, clip paths, masks, gradients (defer to a follow-up)
//   - SMIL animation (Silk's animation system runs imperatively)
//   - external image refs (<image href=...>)
//
// Typical usage:
//
//	doc, err := svg.Parse(data)
//	svg.Render(doc, painter, 24, 24) // render into a 24x24 region
//
// Style inheritance: cascading style is implemented via stack-based
// state during render. A child whose attribute is unspecified inherits
// from its parent group. The standard SVG "presentation attributes" are
// recognised on every shape; the CSS-in-style="" form is also parsed.
//
// Intentionally tiny: we don't pull in a third-party SVG dep. The
// dominant icon-art path is well within the parser's reach, and the
// surface area stays under ~800 LOC so future maintenance is cheap.
package svg
