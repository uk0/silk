package gui

import (
	"regexp"
	"strings"
)

// goSnippets maps abbreviation strings to snippet templates.
// Template placeholders:
//   ${N:text} - replaced with "text" (the default value)
//   ${0}      - replaced with empty string (cursor position marker)
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
