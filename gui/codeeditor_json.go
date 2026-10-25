package gui

// JSON has no valid cross-line strings or comments. Keep the lexer line-local:
// viewport/minimap reads never parse an entire document or require prefix scans.
// This is lexical coloring for incomplete input, not a JSON validator/schema LSP.
// Byte slices preserve Unicode, CR, and even malformed UTF-8 exactly as entered.
func tokenizeJSONLine(line string) []token {
	if len(line) >= maxHighlightLineLength {
		return []token{{text: line, typ: tokNormal}}
	}
	var tokens []token
	add := func(start, end int, kind tokenType) {
		if len(tokens) > 0 && tokens[len(tokens)-1].typ == kind {
			last := &tokens[len(tokens)-1]
			last.text = line[start-len(last.text) : end] // contiguous slice, never quadratic concatenation
		} else {
			tokens = append(tokens, token{text: line[start:end], typ: kind})
		}
	}
	for i := 0; i < len(line); {
		start, kind := i, tokNormal
		switch {
		case line[i] == '"':
			i++
			closed := false
			for i < len(line) {
				ch := line[i]
				i++
				if ch == '\\' && i < len(line) {
					i++
				} else if ch == '"' {
					closed = true
					break
				}
			}
			kind = tokString
			if closed {
				next := i
				for next < len(line) && jsonSpace(line[next]) {
					next++
				}
				if next < len(line) && line[next] == ':' {
					kind = tokType
				}
			}
		case jsonSeparator(line[i]):
			for i < len(line) && jsonSeparator(line[i]) {
				i++
			}
		default:
			for i < len(line) && !jsonSeparator(line[i]) && line[i] != '"' {
				i++
			}
			atom := line[start:i]
			if atom == "true" || atom == "false" || atom == "null" {
				kind = tokKeyword
			} else if jsonNumber(atom) {
				kind = tokNumber
			}
		}
		add(start, i, kind)
	}
	return tokens
}

func jsonSpace(ch byte) bool { return ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' }
func jsonSeparator(ch byte) bool {
	return jsonSpace(ch) || ch == '{' || ch == '}' || ch == '[' || ch == ']' || ch == ',' || ch == ':'
}

// Reject partial numbers, Go numeric syntax, leading zeros and non-finite values.
// No conversion is needed: arbitrarily large JSON integers keep their own text.
func jsonNumber(s string) bool {
	i := 0
	if len(s) > 0 && s[0] == '-' {
		i++
	}
	if i == len(s) {
		return false
	}
	if s[i] == '0' {
		i++
	} else {
		if s[i] < '1' || s[i] > '9' {
			return false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i < len(s) && s[i] == '.' {
		i++
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == start {
			return false
		}
	}
	return i == len(s)
}
