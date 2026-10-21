package gui

import "sort"

// CodeEditor currently has one symbol dialect: the Go heuristic parser. Keep
// the dialect in the key so a future language selector cannot reuse Go results
// for unchanged text in a different language. File extensions do not currently
// change ParseSymbols' behavior; only the test-gutter projection is filename gated.
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
	symbols := parse(lines)
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

// cachedSymbols is the allocation-free read-only path used during drawing.
// The public ParseSymbols method returns a copy instead of exposing this slice.
func (this *CodeEditor) cachedSymbols() []CodeSymbol {
	key := codeSymbolCacheKey{revision: this.TextRevision(), language: codeSymbolLanguageGo}
	return this.symbolCache.load(key, this.lines, func(lines []string) []CodeSymbol {
		symbols := parseGoCodeSymbols(lines)
		if this.additionalSymbolsRevision == this.TextRevision() {
			symbols = append(symbols, this.additionalSymbols...)
			sort.SliceStable(symbols, func(i, j int) bool { return symbols[i].Line < symbols[j].Line })
		}
		return symbols
	})
}
