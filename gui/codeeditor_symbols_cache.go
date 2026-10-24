package gui

import "sort"

// Go and SilkGo share the Go base parser; named non-source files do not invent
// Go declarations. The dialect participates in cache identity independently of text.
const codeSymbolLanguageGo = "go"

type codeSymbolCacheKey struct {
	revision uint64
	language string
}

type codeSymbolCache struct {
	key       codeSymbolCacheKey
	valid     bool // an empty/nil result is also a valid cached parse
	symbols   []CodeSymbol
	functions []int
	testLines map[int]string
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
	c.symbols, c.functions, c.testLines = symbols, functions, tests
	return c.symbols
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
	symbols := e.symbolCache.store(key, mergeCodeSymbols(e.Text(), e.usesGoSyntax(), e.currentAdditionalSymbols()))
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
	if len(additional) > 0 {
		symbols = append(symbols, additional...)
		sort.SliceStable(symbols, func(i, j int) bool { return symbols[i].Line < symbols[j].Line })
	}
	return symbols
}
