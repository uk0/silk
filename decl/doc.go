// Package decl is the declarative authoring layer for Silk widget trees.
//
// The package centres on a small AST type — *Node — that is the canonical
// representation of a UI design. Both the on-disk TDoc format read by the
// designer (.silkui files) and the in-memory Go DSL used by code-first
// authors are projections of the same Node tree:
//
//	Go DSL (.silk.go)  ←→  *decl.Node  ←→  TDoc (.silkui)
//	                          ↓
//	                    factory.New()
//	                          ↓
//	                       IWidget
//
// Why a custom AST and not go/ast?
//
// We need round-tripping between two formats with very different shapes
// (Go function calls vs a tree of name/value pairs). Re-using go/ast as
// the canonical type would force every TDoc node to carry Go syntactic
// fluff, and force every Go author to write only the subset of Go we can
// translate back. A purpose-built AST sidesteps both: TDoc and Go both
// project to the same minimal Node shape, and the projection rules are
// explicit instead of "everything go/ast would let you write minus a
// surprise blocklist".
//
// Layered responsibilities
//
//   - node.go    — Node, Prop, Value sealed interface (Lit / Ref / Bind / Expr)
//   - builder.go — Go DSL: New / Form / VBox / Button / etc. with Attr options
//   - codec_tdoc.go — ToTDoc / FromTDoc round-trip
//   - runtime.go — Node.Build() instantiates IWidget tree via core/factory
//
// # What is NOT in this package
//
// A bidirectional Go-source parser/emitter is intentionally deferred.
// First we prove the AST shape round-trips losslessly with TDoc and the
// runtime (this package). The Go-source codec lives next to it, in a
// future decl/codec_go.go, and only matters when an author wants the
// designer to edit code they hand-wrote.
package decl
