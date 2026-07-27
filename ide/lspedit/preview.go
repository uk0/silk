package lspedit

import (
	"fmt"
	"strings"
)

// Preview rendering. A workspace-wide rename or quick fix is worth showing
// before it lands, which is what Preview turns a Plan into: one entry per file
// with its complete before and after text, produced entirely in memory. Nothing
// here writes, removes or creates anything — a Preview can be discarded and the
// working tree is untouched.

// Action is what a transaction does to one file.
type Action int

// The actions a previewed file can carry.
const (
	ActionModify Action = iota
	ActionCreate
	ActionDelete
)

// String implements fmt.Stringer.
func (a Action) String() string {
	switch a {
	case ActionModify:
		return "modify"
	case ActionCreate:
		return "create"
	case ActionDelete:
		return "delete"
	}
	return fmt.Sprintf("Action(%d)", int(a))
}

// Marker is the single-letter form used in compact listings: M, A or D.
func (a Action) Marker() string {
	switch a {
	case ActionModify:
		return "M"
	case ActionCreate:
		return "A"
	case ActionDelete:
		return "D"
	}
	return "?"
}

// FilePreview is one file's before/after text. Before is empty for a created
// file and After is empty for a deleted one.
type FilePreview struct {
	Path   string
	Action Action
	Before string
	After  string
}

// LineDelta is the change in line count the file undergoes, useful for a
// summary column without diffing the text.
func (f FilePreview) LineDelta() int {
	return lineCount(f.After) - lineCount(f.Before)
}

// Preview is the full set of changes a transaction would make.
type Preview struct {
	Label string
	Files []FilePreview
}

// Empty reports that the transaction changes nothing.
func (p Preview) Empty() bool { return len(p.Files) == 0 }

// Paths lists the affected files in plan order.
func (p Preview) Paths() []string {
	out := make([]string, 0, len(p.Files))
	for _, f := range p.Files {
		out = append(out, f.Path)
	}
	return out
}

// String renders a compact `M path` / `A path` / `D path` listing under the
// transaction label — enough for a confirmation dialog or a log line.
func (p Preview) String() string {
	var b strings.Builder
	if p.Label != "" {
		b.WriteString(p.Label)
		b.WriteString("\n")
	}
	if len(p.Files) == 0 {
		b.WriteString("(no changes)")
		return b.String()
	}
	for i, f := range p.Files {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(f.Action.Marker())
		b.WriteString(" ")
		b.WriteString(f.Path)
	}
	return b.String()
}

// Preview preflights the transaction and returns the resulting per-file
// before/after text. It touches no file: a preflight failure is reported as an
// error exactly as Apply would report it, so a host can validate and show a
// change in one step and only then decide to write.
func (t *Transaction) Preview() (Preview, error) {
	plan, err := t.Preflight()
	if err != nil {
		return Preview{}, err
	}
	pv := Preview{Label: plan.Label, Files: make([]FilePreview, 0, len(plan.Files))}
	for _, f := range plan.Files {
		action := ActionModify
		switch {
		case f.Deleted:
			action = ActionDelete
		case f.Created():
			action = ActionCreate
		}
		pv.Files = append(pv.Files, FilePreview{
			Path:   f.Path,
			Action: action,
			Before: f.Before,
			After:  f.After,
		})
	}
	return pv, nil
}

// lineCount counts the lines of s, treating a trailing newline as terminating
// the last line rather than starting an empty one.
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	n := strings.Count(s, "\n")
	if !strings.HasSuffix(s, "\n") {
		n++
	}
	return n
}
