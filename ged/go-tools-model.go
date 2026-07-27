package ged

import (
	"path/filepath"
	"strconv"

	"github.com/uk0/silk/core"
)

// Pure model behind GoToolsPanel: the picker rows, the tool-grouped
// findings, the flattened row sequence the panel draws and hit-tests, and
// the row labels. No widget, no GL, no process — everything here is a
// free function over plain data, so the panel's layout decisions are
// directly unit-testable and the panel itself only walks the result.

// GoToolRow is one entry in the workflow picker: a core workflow id, the
// executable it shells out to, and whether that executable was found on
// PATH. Unavailable rows still render (so the user can see what a full
// install would offer) but are inert.
type GoToolRow struct {
	Id        string // core.ToolGoVet, core.ToolGovulncheck, ...
	Binary    string // "go", "govulncheck", "staticcheck"
	Available bool
}

// buildGoToolRows turns a PATH-availability snapshot (core.DetectGoTools)
// into picker rows in core.GoToolWorkflows order. Tools missing from the
// snapshot count as unavailable, so a zero snapshot yields a fully greyed
// picker rather than a panel that claims everything is installed.
func buildGoToolRows(tools []core.Tool) []GoToolRow {
	avail := make(map[string]bool, len(tools))
	for _, t := range tools {
		avail[t.Name] = t.Available
	}
	ids := core.GoToolWorkflows()
	out := make([]GoToolRow, 0, len(ids))
	for _, id := range ids {
		bin := core.GoToolBinary(id)
		out = append(out, GoToolRow{Id: id, Binary: bin, Available: bin != "" && avail[bin]})
	}
	return out
}

// GoToolGroup is every finding one tool produced, in the order the parser
// emitted them.
type GoToolGroup struct {
	Tool     string
	Findings []core.Finding
}

// groupFindingsByTool buckets findings by Finding.Tool. Groups come back
// in first-seen order and each group keeps its findings in input order,
// so a run that appends to the pane never reshuffles the rows a user is
// already looking at.
func groupFindingsByTool(findings []core.Finding) []GoToolGroup {
	var out []GoToolGroup
	idx := make(map[string]int)
	for _, f := range findings {
		i, ok := idx[f.Tool]
		if !ok {
			out = append(out, GoToolGroup{Tool: f.Tool})
			i = len(out) - 1
			idx[f.Tool] = i
		}
		out[i].Findings = append(out[i].Findings, f)
	}
	return out
}

// goToolsRowKind tags what a flat row is: a picker entry, a collapsible
// group header, or a finding under one.
type goToolsRowKind int

const (
	goToolsRowPicker goToolsRowKind = iota
	goToolsRowGroup
	goToolsRowFinding
)

// goToolsRow is one drawable line. ToolIdx indexes the picker rows
// (picker kind); GroupIdx indexes the groups and Index the finding within
// its group (group / finding kinds). Unused fields stay zero.
type goToolsRow struct {
	Kind     goToolsRowKind
	ToolIdx  int
	GroupIdx int
	Index    int
}

// buildGoToolsRows flattens the picker plus the grouped findings into the
// single row sequence the panel renders and hit-tests: every picker row
// first, then each group header followed by its findings unless the group
// is collapsed. One flat list for both sections means one row height, one
// hit-test, and one scroll extent.
func buildGoToolsRows(tools []GoToolRow, groups []GoToolGroup, collapsed map[string]bool) []goToolsRow {
	rows := make([]goToolsRow, 0, len(tools)+len(groups))
	for i := range tools {
		rows = append(rows, goToolsRow{Kind: goToolsRowPicker, ToolIdx: i})
	}
	for gi, g := range groups {
		rows = append(rows, goToolsRow{Kind: goToolsRowGroup, GroupIdx: gi})
		if collapsed[g.Tool] {
			continue
		}
		for fi := range g.Findings {
			rows = append(rows, goToolsRow{Kind: goToolsRowFinding, GroupIdx: gi, Index: fi})
		}
	}
	return rows
}

// goToolsRowAtY maps a y coordinate to a flat-row index for rows starting
// at topOffset with height rowH. The caller folds the scroll offset into
// y. Returns -1 above the rows (the header band), past the last row, or
// for a degenerate rowH. (Named goToolsRowAtY because git-changes-panel.go
// already owns a package-level rowAtY.)
func goToolsRowAtY(y, topOffset, rowH float64, count int) int {
	if rowH <= 0 || y < topOffset {
		return -1
	}
	idx := int((y - topOffset) / rowH)
	if idx < 0 || idx >= count {
		return -1
	}
	return idx
}

// goToolsHeaderLabel renders the header tally, e.g.
// "Go 工具 / Go Tools (5)".
func goToolsHeaderLabel(findings int) string {
	return "Go 工具 / Go Tools (" + strconv.Itoa(findings) + ")"
}

// goToolPickerLabel renders a picker row: the workflow id, plus which
// binary is missing when the workflow cannot run. Naming the binary
// rather than saying "unavailable" tells the user what to install.
func goToolPickerLabel(r GoToolRow) string {
	if r.Available {
		return r.Id
	}
	if r.Binary == "" {
		return r.Id + " (unknown tool)"
	}
	return r.Id + " (" + r.Binary + " not on PATH)"
}

// goToolGroupLabel renders a group header: the expand glyph, the tool id
// and its finding count, e.g. "▼ go vet (3)".
func goToolGroupLabel(g GoToolGroup, collapsed bool) string {
	glyph := "▼"
	if collapsed {
		glyph = "▶"
	}
	return glyph + " " + g.Tool + " (" + strconv.Itoa(len(g.Findings)) + ")"
}

// goToolFindingLabel renders a finding's left-hand locator as
// "basename:line[:col]", dropping the directory so the list stays
// scannable. Findings with no source location — a `pprof -top` row at
// function granularity, a module-level govulncheck hit — yield "", which
// is also how the panel decides a row cannot be jumped to.
func goToolFindingLabel(f core.Finding) string {
	if f.File == "" {
		return ""
	}
	s := filepath.Base(f.File) + ":" + strconv.Itoa(f.Line)
	if f.Col > 0 {
		s += ":" + strconv.Itoa(f.Col)
	}
	return s
}

// goToolFindingText renders a finding's message, prefixed with the tool's
// own identifier when it reported one ("SA4006: ...", "GO-2024-2687: ...")
// so the row carries the code the user would search for.
func goToolFindingText(f core.Finding) string {
	if f.Code == "" {
		return f.Message
	}
	return f.Code + ": " + f.Message
}
