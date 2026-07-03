package gui

import (
	"regexp"
	"strings"
)

// goSnippets maps abbreviation strings to snippet templates.
// Template placeholders:
//
//	${N:text} - replaced with "text" (the default value)
//	${0}      - replaced with empty string (cursor position marker)
var goSnippets = map[string]string{
	"fn":    "func ${1:name}(${2:params}) ${3:returnType} {\n\t${0}\n}",
	"fne":   "func (${1:err} error) {\n\t${0}\n}",
	"if":    "if ${1:condition} {\n\t${0}\n}",
	"ife":   "if err != nil {\n\t${0}\n}",
	"for":   "for ${1:i} := 0; ${1:i} < ${2:n}; ${1:i}++ {\n\t${0}\n}",
	"forr":  "for ${1:k}, ${2:v} := range ${3:collection} {\n\t${0}\n}",
	"sw":    "switch ${1:value} {\ncase ${2:case1}:\n\t${0}\ndefault:\n}",
	"pr":    "fmt.Println(${0})",
	"prf":   "fmt.Printf(\"${1}\\n\", ${0})",
	"err":   "if err != nil {\n\treturn err\n}",
	"main":  "func main() {\n\t${0}\n}",
	"st":    "type ${1:Name} struct {\n\t${0}\n}",
	"iface": "type ${1:Name} interface {\n\t${0}\n}",
	"meth":  "func (this *${1:Type}) ${2:Name}(${3:params}) ${4:returns} {\n\t${0}\n}",
}

// snippetPlaceholderRe matches ${N:text} placeholders in snippet templates.
var snippetPlaceholderRe = regexp.MustCompile(`\$\{(\d+)(?::([^}]*))?}`)

// expandSnippet takes a snippet template and returns the expanded text
// with all placeholders replaced by their default values.
func expandSnippet(template string) string {
	result := snippetPlaceholderRe.ReplaceAllStringFunc(template, func(match string) string {
		sub := snippetPlaceholderRe.FindStringSubmatch(match)
		if len(sub) < 2 {
			return match
		}
		// sub[1] is the number, sub[2] is the default text (may be empty)
		num := sub[1]
		defaultText := ""
		if len(sub) >= 3 {
			defaultText = sub[2]
		}
		// ${0} becomes empty string (cursor position)
		if num == "0" {
			return ""
		}
		return defaultText
	})
	return result
}

// tryExpandSnippet checks if the word before the cursor matches a snippet
// abbreviation, and if so, replaces it with the expanded snippet.
// Returns true if a snippet was expanded.
func (this *CodeEditor) tryExpandSnippet() bool {
	if this.cursorLine >= len(this.lines) {
		return false
	}
	runes := []rune(this.lines[this.cursorLine])
	end := this.cursorCol
	if end > len(runes) {
		end = len(runes)
	}

	// Walk backwards to find the start of the word
	start := end
	for start > 0 && isIdentPart(runes[start-1]) {
		start--
	}
	if start == end {
		return false
	}

	word := string(runes[start:end])
	template, ok := goSnippets[word]
	if !ok {
		return false
	}

	// Expand the snippet
	expanded := expandSnippet(template)

	// Record undo: replace the abbreviation with the expanded text
	this.pushUndo(editAction{kind: 2, line: this.cursorLine, col: start, text: expanded, oldText: word})

	// Delete the abbreviation text
	newRunes := append(runes[:start], runes[end:]...)
	this.lines[this.cursorLine] = string(newRunes)
	this.cursorCol = start

	// Insert the expanded snippet (may be multi-line)
	expandedLines := strings.Split(expanded, "\n")

	// Apply indentation: use the current line's indentation for each new line
	indent := ""
	for _, r := range runes[:start] {
		if r == ' ' || r == '\t' {
			indent += string(r)
		} else {
			break
		}
	}

	if len(expandedLines) == 1 {
		this.insertTextAtCursor(expanded)
	} else {
		// Add indent to lines after the first
		for i := 1; i < len(expandedLines); i++ {
			expandedLines[i] = indent + expandedLines[i]
		}
		this.insertMultilineAtCursor(expandedLines)
	}

	this.rebuildText()
	this.ensureCursorVisible()
	this.Self().Update()
	return true
}

// ---------------------------------------------------------------------------
// Public snippet API (Qt Creator-style)
//
// Unlike the internal goSnippets table above (which uses ${N:text}/${0}
// placeholders for the live CodeEditor), the public Snippet/SnippetSet model
// uses the simpler "$0" cursor mark only. Callers integrate it by:
//   1. Reading the word ending at the cursor.
//   2. Calling Expand(buffer, cursor, trigger) on Tab/Enter.
//   3. Replacing the buffer + moving the cursor to the returned offset.
//
// The internal helper machinery above is kept untouched so existing
// CodeEditor wiring continues to work; this API is editor-agnostic.
// ---------------------------------------------------------------------------

// Snippet is a single trigger-to-body template.
//
// Body may contain a single "$0" mark indicating the final cursor position
// after expansion. The mark is stripped from the inserted text.
type Snippet struct {
	Trigger string // e.g. "iferr"
	Title   string // human label, e.g. "if err != nil"
	Body    string // template body; may contain "$0" cursor mark
}

// SnippetSet is an ordered, lookup-friendly bundle of Snippets.
type SnippetSet struct {
	items  []*Snippet
	byTrig map[string]*Snippet
}

// NewGoSnippetSet returns the default Go snippet set.
func NewGoSnippetSet() *SnippetSet {
	s := &SnippetSet{byTrig: make(map[string]*Snippet)}
	s.add(&Snippet{Trigger: "iferr", Title: "if err != nil { return err }",
		Body: "if err != nil {\n\treturn err$0\n}"})
	s.add(&Snippet{Trigger: "iferrln", Title: "if err != nil { log.Fatal(err) }",
		Body: "if err != nil {\n\tlog.Fatal(err)$0\n}"})
	s.add(&Snippet{Trigger: "forrange", Title: "for k, v := range",
		Body: "for k, v := range $0 {\n\t\n}"})
	s.add(&Snippet{Trigger: "forr", Title: "for i := 0; i < n; i++",
		Body: "for i := 0; i < $0; i++ {\n\t\n}"})
	s.add(&Snippet{Trigger: "func", Title: "func declaration",
		Body: "func $0() {\n\t\n}"})
	s.add(&Snippet{Trigger: "Test", Title: "testing.T function",
		Body: "func Test$0(t *testing.T) {\n\t\n}"})
	s.add(&Snippet{Trigger: "Benchmark", Title: "testing.B function",
		Body: "func Benchmark$0(b *testing.B) {\n\tfor i := 0; i < b.N; i++ {\n\t\t\n\t}\n}"})
	s.add(&Snippet{Trigger: "sprintf", Title: "fmt.Sprintf",
		Body: "fmt.Sprintf(\"$0\", )"})
	s.add(&Snippet{Trigger: "errwrap", Title: "fmt.Errorf with %w",
		Body: "fmt.Errorf(\"$0: %w\", err)"})
	return s
}

func (this *SnippetSet) add(sn *Snippet) {
	this.items = append(this.items, sn)
	this.byTrig[sn.Trigger] = sn
}

// ByTrigger returns the snippet for trigger, or nil if none.
func (this *SnippetSet) ByTrigger(trigger string) *Snippet {
	if this == nil {
		return nil
	}
	return this.byTrig[trigger]
}

// Triggers returns the trigger strings in registration order.
func (this *SnippetSet) Triggers() []string {
	if this == nil {
		return nil
	}
	out := make([]string, 0, len(this.items))
	for _, sn := range this.items {
		out = append(out, sn.Trigger)
	}
	return out
}

// Expand attempts to expand the trigger ending at cursor in buffer.
//
// Preconditions for expansion (all required):
//   - The last len([]rune(trigger)) runes before cursor equal trigger.
//   - The rune immediately left of the trigger is either start-of-buffer,
//     a newline, or a non-identifier character (so "xiferr" doesn't match
//     "iferr").
//   - SnippetSet has a snippet registered for trigger.
//
// On match, the trigger text is removed and the snippet body is inserted in
// its place. The leading whitespace of the current line is captured and
// prefixed to every body line AFTER the first, so the expansion lines up
// with the trigger column. The "$0" cursor mark, if present, is stripped
// from the inserted text and newCursor is positioned where it sat;
// otherwise newCursor lands at the end of the inserted body.
//
// Returns ok=false (and unchanged buffer/cursor) when no snippet matches.
func (this *SnippetSet) Expand(buffer string, cursor int, trigger string) (string, int, bool) {
	if this == nil {
		return buffer, cursor, false
	}
	sn := this.byTrig[trigger]
	if sn == nil {
		return buffer, cursor, false
	}
	runes := []rune(buffer)
	if cursor < 0 || cursor > len(runes) {
		return buffer, cursor, false
	}
	trigRunes := []rune(trigger)
	wordLen := len(trigRunes)
	if wordLen == 0 || cursor < wordLen {
		return buffer, cursor, false
	}
	// Confirm the trigger sits flush against the cursor.
	start := cursor - wordLen
	for i, r := range trigRunes {
		if runes[start+i] != r {
			return buffer, cursor, false
		}
	}
	// Left boundary: SOL or non-identifier char.
	if start > 0 {
		prev := runes[start-1]
		if prev != '\n' && isIdentPart(prev) {
			return buffer, cursor, false
		}
	}

	// Capture leading whitespace of the current line (everything from the
	// last newline before `start` up to the first non-ws rune).
	lineStart := start
	for lineStart > 0 && runes[lineStart-1] != '\n' {
		lineStart--
	}
	indent := ""
	for i := lineStart; i < start; i++ {
		if runes[i] == ' ' || runes[i] == '\t' {
			indent += string(runes[i])
		} else {
			break
		}
	}

	// Prefix indent to body lines after the first.
	body := sn.Body
	lines := strings.Split(body, "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = indent + lines[i]
	}
	indented := strings.Join(lines, "\n")

	// Locate and strip the $0 cursor mark; offset is relative to indented body.
	cursorInBody := strings.Index(indented, "$0")
	if cursorInBody >= 0 {
		indented = indented[:cursorInBody] + indented[cursorInBody+len("$0"):]
	} else {
		cursorInBody = len([]rune(indented))
	}

	// Splice: drop the trigger word, insert the indented body.
	before := string(runes[:start])
	after := string(runes[cursor:])
	newBuffer := before + indented + after
	// newCursor is start + rune count of indented[:cursorInBody].
	newCursor := start + len([]rune(indented[:cursorInBody]))
	return newBuffer, newCursor, true
}
