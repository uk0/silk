package gui

import (
	"slices"
	"sort"
)

// Go and SilkGo share the Go base parser; named non-source files do not invent
// Go declarations. The dialect participates in cache identity independently of text.
const codeSymbolLanguageGo = "go"

type codeSymbolCacheKey struct {
	revision uint64
	language string
}

type codeSymbolCache struct {
	key             codeSymbolCacheKey
	valid           bool                // an empty/nil result is also a valid cached parse
	contentRevision uint64              // zero is reserved for an unobserved snapshot
	projection      *goSymbolProjection // immutable, base Go symbols only
	symbols         []CodeSymbol
	functions       []int
	testLines       map[int]string
}

// load owns the parser result and its immutable derived indexes. The parser
// argument keeps the cache independently testable without native drawing or a
// production instrumentation counter. Callers must treat the result as read-only.
func (c *codeSymbolCache) load(key codeSymbolCacheKey, lines []string, parse func([]string) []CodeSymbol) []CodeSymbol {
	if c.valid && c.key == key {
		return c.symbols
	}
	return c.store(key, parse(lines))
}

func (c *codeSymbolCache) store(key codeSymbolCacheKey, symbols []CodeSymbol) []CodeSymbol {
	c.projection = nil
	if c.contentRevision != 0 && slices.Equal(c.symbols, symbols) {
		c.key, c.valid = key, true
		return c.symbols
	}
	var functions []int
	var tests map[int]string
	for i, symbol := range symbols {
		if symbol.Kind == SymFunc || symbol.Kind == SymMethod {
			functions = append(functions, i)
		}
		if symbol.Kind == SymFunc && isGoTestFuncName(symbol.Name) {
			if tests == nil {
				tests = make(map[int]string)
			}
			tests[symbol.Line] = symbol.Name
		}
	}
	c.key, c.valid = key, true
	c.contentRevision = nextSymbolContentRevision(c.contentRevision)
	c.symbols, c.functions, c.testLines = symbols, functions, tests
	return c.symbols
}

func nextSymbolContentRevision(previous uint64) uint64 {
	next := previous + 1
	if next == 0 {
		next = 1
	}
	return next
}

// Adopt a worker-owned immutable result without losing the editor's content
// identity. Equal declarations retain their derived indexes and storage too.
func (c *codeSymbolCache) adopt(next codeSymbolCache) {
	if c.contentRevision != 0 && next.valid && slices.Equal(c.symbols, next.symbols) {
		c.key, c.valid = next.key, true
		c.projection = next.projection
		return
	}
	next.contentRevision = nextSymbolContentRevision(c.contentRevision)
	*c = next
}

// functionAt returns the last function/method at or before a source line.
// parseGoCodeSymbols emits declarations in source order, so binary search is
// sufficient. The pointer is private and read-only; ParseSymbols copies exports.
func (c *codeSymbolCache) functionAt(line int) *CodeSymbol {
	lo, hi := 0, len(c.functions)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if c.symbols[c.functions[mid]].Line <= line {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	if lo == 0 {
		return nil
	}
	return &c.symbols[c.functions[lo-1]]
}

// cachedSymbols is the synchronous compatibility path for explicit navigation.
// Drawing and outline refreshes use ensureSymbols instead.
func (e *CodeEditor) cachedSymbols() []CodeSymbol {
	key := codeSymbolCacheKey{revision: e.TextRevision(), language: e.Language()}
	if e.symbolCache.valid && e.symbolCache.key == key {
		return e.symbolCache.symbols
	}
	pending := e.symbolPending
	e.cancelSymbolRequest()
	// Explicit SDK queries and the small-file synchronous path must publish
	// the same exact immutable metadata as the large-file background worker.
	var index goSymbolIndex
	var base []CodeSymbol
	if e.usesGoSyntax() {
		base = index.parse(e.Text())
	}
	additional := e.currentAdditionalSymbols()
	symbols := e.symbolCache.store(key, mergeAdditionalCodeSymbols(base, additional))
	if len(additional) == 0 {
		e.symbolCache.projection = index.projection()
	}
	if pending {
		e.symbolsRevision++
		e.notifySymbolsChanged()
	} // wake a nonblocking outline waiting on the canceled request
	return symbols
}

func (e *CodeEditor) currentAdditionalSymbols() []CodeSymbol {
	if e.additionalSymbolsRevision == e.TextRevision() {
		return e.additionalSymbols
	}
	return nil
}

func mergeCodeSymbols(text string, goSyntax bool, additional []CodeSymbol) []CodeSymbol {
	var symbols []CodeSymbol
	if goSyntax {
		symbols = parseGoCodeSymbolsText(text)
	}
	return mergeAdditionalCodeSymbols(symbols, additional)
}

func mergeAdditionalCodeSymbols(symbols, additional []CodeSymbol) []CodeSymbol {
	if len(additional) > 0 {
		// The base can be shared with the incremental index or a published
		// cache. Appending/sorting must never mutate that immutable projection.
		combined := make([]CodeSymbol, 0, len(symbols)+len(additional))
		symbols = append(append(combined, symbols...), additional...)
		sort.SliceStable(symbols, func(i, j int) bool { return symbols[i].Line < symbols[j].Line })
	}
	return symbols
}
