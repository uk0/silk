package core

import (
	"bufio"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Multi-file patch model: a unified diff parsed into an editable, appliable
// shape, plus the operations an IDE needs on top of it (apply, partial /
// per-hunk apply for staging, reverse for revert, and a whole-file
// reconstruction that keeps the unchanged gaps between hunks).
//
// Relationship to gitdiff.go: DiffFile/DiffHunk/DiffLine are the *render*
// model — "which lines did git report", enough to paint editor gutter marks.
// PatchSet is the *edit* model: it also carries rename/add/delete identity
// and the no-newline-at-end-of-file bit, and it can produce new file content.
// The two coexist on purpose; nothing here changes ParseUnifiedDiff, and the
// low-level header/path helpers (parseHunkHeader, parseDiffPath) are shared.
//
// Everything in this file is pure: no I/O, no globals, never panics. A
// malformed @@ header is skipped and reported through the returned error
// while the rest of the patch still parses.

// PatchLineKind classifies one line inside a hunk body.
type PatchLineKind int

const (
	PatchContext PatchLineKind = iota // " " unchanged, present on both sides
	PatchAdded                        // "+" present only in the new file
	PatchDeleted                      // "-" present only in the old file
)

// String renders the kind's diff prefix name, for error messages.
func (k PatchLineKind) String() string {
	switch k {
	case PatchAdded:
		return "added"
	case PatchDeleted:
		return "deleted"
	default:
		return "context"
	}
}

// PatchLine is one line of a hunk body. Text excludes the leading +/-/space
// marker. NoNewline records that a "\ No newline at end of file" marker
// followed this line — i.e. this line is the last line of its side and it is
// not terminated by a newline. The marker is a property of the line before
// it, not a line of its own, so it never occupies a slot in either file.
type PatchLine struct {
	Kind      PatchLineKind
	Text      string
	NoNewline bool
}

// PatchHunk is one "@@ -OldStart,OldLines +NewStart,NewLines @@" block.
// Starts are 1-based; a count omitted in the header means 1 (git's shorthand
// for a single-line range). A creation hunk reads "@@ -0,0 +1,N @@", i.e.
// OldStart 0 / OldLines 0.
type PatchHunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Section  string // text after the closing "@@" (function context), may be empty
	Lines    []PatchLine
}

// FilePatch is every hunk for one file. Paths have the a//b/ prefix stripped
// and /dev/null normalised to "": OldPath == "" is a new file, NewPath == ""
// a deleted one, and two different non-empty paths a rename.
type FilePatch struct {
	OldPath string
	NewPath string
	Hunks   []PatchHunk
}

// PatchSet is a whole unified diff — every file it touches, in diff order.
type PatchSet struct {
	Files []FilePatch
}

// IsAdd reports whether the patch creates the file (old side is /dev/null).
func (f FilePatch) IsAdd() bool { return f.OldPath == "" && f.NewPath != "" }

// IsDelete reports whether the patch deletes the file (new side is /dev/null).
func (f FilePatch) IsDelete() bool { return f.NewPath == "" && f.OldPath != "" }

// IsRename reports whether the file moved: both sides exist and differ.
func (f FilePatch) IsRename() bool {
	return f.OldPath != "" && f.NewPath != "" && f.OldPath != f.NewPath
}

// Path is the path to show for the patch: the new path when the file still
// exists, otherwise the old one (a deletion).
func (f FilePatch) Path() string {
	if f.NewPath != "" {
		return f.NewPath
	}
	return f.OldPath
}

// Header re-renders the hunk's "@@ ... @@" line, counts included except for
// the ",1" git elides. Used as the header row of a hunk in a diff viewer.
func (h PatchHunk) Header() string {
	s := "@@ -" + patchRangeText(h.OldStart, h.OldLines) +
		" +" + patchRangeText(h.NewStart, h.NewLines) + " @@"
	if h.Section != "" {
		s += " " + h.Section
	}
	return s
}

// patchRangeText renders "start,count", dropping the count when it is 1.
func patchRangeText(start, count int) string {
	if count == 1 {
		return strconv.Itoa(start)
	}
	return strconv.Itoa(start) + "," + strconv.Itoa(count)
}

// Stats counts the added and deleted lines of the hunk.
func (h PatchHunk) Stats() (added, deleted int) {
	for _, ln := range h.Lines {
		switch ln.Kind {
		case PatchAdded:
			added++
		case PatchDeleted:
			deleted++
		}
	}
	return added, deleted
}

// Reverse flips the hunk so applying it undoes the original: the two ranges
// swap, additions become deletions and vice versa, context lines stay put.
// The no-newline bit rides along with the line it was attached to, which is
// exactly what a round-trip needs (the line that had no terminator on one
// side is the line that has none on the other).
func (h PatchHunk) Reverse() PatchHunk {
	out := PatchHunk{
		OldStart: h.NewStart,
		OldLines: h.NewLines,
		NewStart: h.OldStart,
		NewLines: h.OldLines,
		Section:  h.Section,
	}
	if len(h.Lines) > 0 {
		out.Lines = make([]PatchLine, len(h.Lines))
	}
	for i, ln := range h.Lines {
		switch ln.Kind {
		case PatchAdded:
			ln.Kind = PatchDeleted
		case PatchDeleted:
			ln.Kind = PatchAdded
		}
		out.Lines[i] = ln
	}
	return out
}

// Reverse flips every hunk of the file patch and swaps its paths, turning an
// "apply" patch into a "revert" patch. Hunk order is preserved: reversed
// coordinates are already in new-file space, which is the file the reverse
// patch applies to.
func (f FilePatch) Reverse() FilePatch {
	out := FilePatch{OldPath: f.NewPath, NewPath: f.OldPath}
	if len(f.Hunks) > 0 {
		out.Hunks = make([]PatchHunk, len(f.Hunks))
	}
	for i, h := range f.Hunks {
		out.Hunks[i] = h.Reverse()
	}
	return out
}

// Apply applies every hunk of the file patch to original.
func (f FilePatch) Apply(original string) (string, error) {
	return ApplyPatch(original, f.Hunks)
}

// ApplySelected applies only the hunks at hunkIndices — the partial-staging
// primitive ("stage this hunk"). Indices refer to f.Hunks; order and
// duplicates do not matter.
func (f FilePatch) ApplySelected(original string, hunkIndices []int) (string, error) {
	return ApplyPatchSelected(original, f.Hunks, hunkIndices)
}

// RebuildSides reconstructs both complete sides of the patch from the
// original file content: the old side is original verbatim, the new side is
// original with every hunk applied. Unlike concatenating hunk bodies, this
// keeps the unchanged gaps between hunks, so a side-by-side viewer can show
// the whole file instead of only the changed neighbourhoods.
func (f FilePatch) RebuildSides(original string) (oldText, newText string, err error) {
	return RebuildPatchSides(original, f.Hunks)
}

// RebuildPatchSides is RebuildSides for a bare hunk slice.
func RebuildPatchSides(original string, hunks []PatchHunk) (oldText, newText string, err error) {
	newText, err = ApplyPatch(original, hunks)
	if err != nil {
		return "", "", err
	}
	return original, newText, nil
}

// ApplyPatch applies hunks, in order, to original and returns the new file
// content. Every context and deleted line is validated against the original
// at the line the hunk claims it sits on; the first mismatch aborts with an
// error naming the line, so a stale patch can never silently corrupt a file.
// An empty hunk slice returns original unchanged.
//
// Trailing newline: the result keeps the original's terminator unless the
// patch replaces the final line, in which case the new final line's
// NoNewline bit decides. That makes Apply/Reverse an exact round-trip across
// the "\ No newline at end of file" marker.
func ApplyPatch(original string, hunks []PatchHunk) (string, error) {
	lines, trailing := splitPatchLines(original)

	out := make([]string, 0, len(lines)+8)
	cursor := 0   // next original line (0-based) not yet consumed
	lastIdx := -1 // index in out of the last line emitted from a hunk
	lastNoNL := false

	for hi, h := range hunks {
		start := h.OldStart - 1
		if h.OldStart <= 0 {
			// "@@ -0,0 +1,N @@": creation hunk, nothing precedes it.
			start = 0
		}
		if start > len(lines) {
			return "", fmt.Errorf("patch hunk %d: old start line %d is past the end of the file (%d lines)",
				hi+1, h.OldStart, len(lines))
		}
		if start < cursor {
			return "", fmt.Errorf("patch hunk %d: old start line %d overlaps the previous hunk (already at line %d)",
				hi+1, h.OldStart, cursor+1)
		}
		// Untouched gap before the hunk.
		out = append(out, lines[cursor:start]...)
		cursor = start

		for _, ln := range h.Lines {
			switch ln.Kind {
			case PatchContext, PatchDeleted:
				if cursor >= len(lines) {
					return "", fmt.Errorf("patch hunk %d: %v line %q runs past the end of the file (%d lines)",
						hi+1, ln.Kind, ln.Text, len(lines))
				}
				if lines[cursor] != ln.Text {
					return "", fmt.Errorf("patch hunk %d: %v mismatch at line %d: file has %q, patch expects %q",
						hi+1, ln.Kind, cursor+1, lines[cursor], ln.Text)
				}
				if ln.Kind == PatchContext {
					out = append(out, ln.Text)
					lastIdx, lastNoNL = len(out)-1, ln.NoNewline
				}
				cursor++
			case PatchAdded:
				out = append(out, ln.Text)
				lastIdx, lastNoNL = len(out)-1, ln.NoNewline
			}
		}
	}

	tail := cursor
	out = append(out, lines[tail:]...)

	if len(out) == 0 {
		return "", nil
	}
	// The patch only gets to decide the terminator when its own last line is
	// the file's last line; otherwise the untouched tail keeps the original's.
	if tail >= len(lines) && lastIdx == len(out)-1 {
		trailing = !lastNoNL
	}
	s := strings.Join(out, "\n")
	if trailing {
		s += "\n"
	}
	return s, nil
}

// ApplyPatchSelected applies only the hunks at hunkIndices. Because every
// hunk's coordinates are relative to the original file, skipping a hunk needs
// no offset fixups — this is what makes per-hunk staging work. Indices may
// arrive in any order and may repeat; they are de-duplicated and sorted. An
// out-of-range index is an error (nothing is applied). No indices at all
// returns original unchanged.
func ApplyPatchSelected(original string, hunks []PatchHunk, hunkIndices []int) (string, error) {
	sel, err := selectPatchHunks(hunks, hunkIndices)
	if err != nil {
		return "", err
	}
	return ApplyPatch(original, sel)
}

// selectPatchHunks resolves hunkIndices into a de-duplicated, ascending hunk
// slice, or an error naming the first out-of-range index.
func selectPatchHunks(hunks []PatchHunk, hunkIndices []int) ([]PatchHunk, error) {
	if len(hunkIndices) == 0 {
		return nil, nil
	}
	seen := make(map[int]bool, len(hunkIndices))
	ordered := make([]int, 0, len(hunkIndices))
	for _, i := range hunkIndices {
		if i < 0 || i >= len(hunks) {
			return nil, fmt.Errorf("hunk index %d out of range (%d hunks)", i, len(hunks))
		}
		if seen[i] {
			continue
		}
		seen[i] = true
		ordered = append(ordered, i)
	}
	sort.Ints(ordered)
	out := make([]PatchHunk, 0, len(ordered))
	for _, i := range ordered {
		out = append(out, hunks[i])
	}
	return out, nil
}

// splitPatchLines splits file content into lines and reports whether the
// content ended with a newline. "" is zero lines (not one empty line) so a
// patch that creates a file starts from nothing.
func splitPatchLines(s string) (lines []string, trailingNewline bool) {
	if s == "" {
		return nil, false
	}
	if strings.HasSuffix(s, "\n") {
		return strings.Split(strings.TrimSuffix(s, "\n"), "\n"), true
	}
	return strings.Split(s, "\n"), false
}

// ParsePatchSet parses a unified diff (`git diff`, `diff -u`) into a PatchSet.
// Handles multiple files, "diff --git" / "rename from|to" identity, /dev/null
// add and delete sides, "@@" ranges with elided counts, and the
// "\ No newline at end of file" marker.
//
// Hunk bodies are consumed by the counts in their own @@ header rather than by
// sniffing the first character. That is what makes a deleted source line like
// "-- x" (which reaches the diff as "--- x") parse as a deleted line instead
// of being mistaken for a "--- path" file header.
//
// A malformed @@ header drops that one hunk and is reported through the
// returned error; every other file and hunk still comes back in the PatchSet.
// Empty input yields a zero PatchSet and no error.
func ParsePatchSet(src string) (PatchSet, error) {
	var ps PatchSet
	if strings.TrimSpace(src) == "" {
		return ps, nil
	}

	var (
		errs    []string
		cur     *FilePatch
		hunk    *PatchHunk
		oldLeft int  // old-side body lines still expected in the open hunk
		newLeft int  // new-side body lines still expected in the open hunk
		gitHdr  bool // current file started from a "diff --git" line
	)

	flushHunk := func() {
		if cur != nil && hunk != nil {
			cur.Hunks = append(cur.Hunks, *hunk)
		}
		hunk = nil
		oldLeft, newLeft = 0, 0
	}
	flushFile := func() {
		flushHunk()
		if cur != nil {
			ps.Files = append(ps.Files, *cur)
		}
		cur = nil
		gitHdr = false
	}

	scanner := bufio.NewScanner(strings.NewReader(src))
	// Diff lines are short in practice, but minified sources happen; match
	// ParseUnifiedDiff's 4 MiB ceiling.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()

		if hunk != nil {
			if oldLeft > 0 || newLeft > 0 {
				if consumePatchBody(hunk, line, &oldLeft, &newLeft) {
					continue
				}
				// Not a body line while lines are still owed: truncated hunk.
				// Close it and re-dispatch this line as a header.
				flushHunk()
			} else if strings.HasPrefix(line, "\\") {
				// Marker for the hunk's final line, after both counts ran out.
				markPatchNoNewline(hunk)
				continue
			}
		}

		switch {
		case strings.HasPrefix(line, "diff --git "):
			flushFile()
			cur = &FilePatch{}
			gitHdr = true
			if o, n, ok := parseGitDiffPaths(strings.TrimPrefix(line, "diff --git ")); ok {
				cur.OldPath, cur.NewPath = o, n
			}

		case strings.HasPrefix(line, "rename from "):
			if cur == nil {
				cur = &FilePatch{}
			}
			cur.OldPath = parseDiffPath(strings.TrimPrefix(line, "rename from "))

		case strings.HasPrefix(line, "rename to "):
			if cur == nil {
				cur = &FilePatch{}
			}
			cur.NewPath = parseDiffPath(strings.TrimPrefix(line, "rename to "))

		case strings.HasPrefix(line, "--- "):
			// Plain `diff -u` output has no "diff --git" line, so "--- " has
			// to double as the file boundary there. Inside a git patch the
			// paths are already known, so only accumulated hunks mark a new
			// file — otherwise the "--- " right after "diff --git" would
			// split the same file in two.
			switch {
			case cur == nil:
				cur = &FilePatch{}
			case len(cur.Hunks) > 0, !gitHdr && (cur.OldPath != "" || cur.NewPath != ""):
				flushFile()
				cur = &FilePatch{}
			}
			flushHunk()
			cur.OldPath = parseDiffPath(strings.TrimPrefix(line, "--- "))

		case strings.HasPrefix(line, "+++ "):
			if cur == nil {
				cur = &FilePatch{}
			}
			flushHunk()
			cur.NewPath = parseDiffPath(strings.TrimPrefix(line, "+++ "))

		case strings.HasPrefix(line, "@@"):
			flushHunk()
			h, err := parsePatchHunkHeader(line)
			if err != nil {
				errs = append(errs, fmt.Sprintf("line %d: %v", lineNo, err))
				continue
			}
			if cur == nil {
				cur = &FilePatch{}
			}
			hunk = &h
			oldLeft, newLeft = h.OldLines, h.NewLines

		default:
			// "index abc..def 100644", "new file mode ...", "similarity
			// index ...", "Binary files ...": metadata the model does not
			// need — the paths and hunks carry everything.
		}
	}
	flushFile()

	if err := scanner.Err(); err != nil {
		errs = append(errs, fmt.Sprintf("scan: %v", err))
	}
	if len(errs) > 0 {
		return ps, fmt.Errorf("patch set parse: %s", strings.Join(errs, "; "))
	}
	return ps, nil
}

// consumePatchBody appends line to the open hunk when it is a body line,
// decrementing the per-side budgets. It reports false when the line cannot
// belong to the body (so the caller re-reads it as a header line).
func consumePatchBody(h *PatchHunk, line string, oldLeft, newLeft *int) bool {
	if line == "" {
		// Some tools strip the single trailing space of an empty context line.
		if *oldLeft <= 0 || *newLeft <= 0 {
			return false
		}
		h.Lines = append(h.Lines, PatchLine{Kind: PatchContext})
		*oldLeft--
		*newLeft--
		return true
	}
	switch line[0] {
	case ' ':
		if *oldLeft <= 0 || *newLeft <= 0 {
			return false
		}
		h.Lines = append(h.Lines, PatchLine{Kind: PatchContext, Text: line[1:]})
		*oldLeft--
		*newLeft--
		return true
	case '+':
		if *newLeft <= 0 {
			return false
		}
		h.Lines = append(h.Lines, PatchLine{Kind: PatchAdded, Text: line[1:]})
		*newLeft--
		return true
	case '-':
		if *oldLeft <= 0 {
			return false
		}
		h.Lines = append(h.Lines, PatchLine{Kind: PatchDeleted, Text: line[1:]})
		*oldLeft--
		return true
	case '\\':
		// "\ No newline at end of file" — a property of the previous line,
		// costing neither side a line.
		markPatchNoNewline(h)
		return true
	}
	return false
}

// markPatchNoNewline flags the hunk's most recent line as unterminated.
func markPatchNoNewline(h *PatchHunk) {
	if n := len(h.Lines); n > 0 {
		h.Lines[n-1].NoNewline = true
	}
}

// parsePatchHunkHeader parses "@@ -A[,B] +C[,D] @@ [section]", reusing
// gitdiff.go's range parser and additionally keeping the section text.
func parsePatchHunkHeader(line string) (PatchHunk, error) {
	dh, err := parseHunkHeader(line)
	if err != nil {
		return PatchHunk{}, err
	}
	h := PatchHunk{
		OldStart: dh.OldStart,
		OldLines: dh.OldCount,
		NewStart: dh.NewStart,
		NewLines: dh.NewCount,
	}
	// Anything after the closing "@@" is the enclosing-function hint.
	if i := strings.Index(line[2:], "@@"); i >= 0 {
		h.Section = strings.TrimSpace(line[2+i+2:])
	}
	return h, nil
}

// parseGitDiffPaths splits the "a/old b/new" tail of a "diff --git" line.
// The two paths are separated by " b/", which is searched from the right so a
// path containing spaces still resolves. Falls back to a plain two-field
// split for --no-prefix output; reports false when the tail is ambiguous.
func parseGitDiffPaths(rest string) (oldPath, newPath string, ok bool) {
	if i := strings.LastIndex(rest, " b/"); i >= 0 {
		return parseDiffPath(rest[:i]), parseDiffPath(rest[i+1:]), true
	}
	fields := strings.Fields(rest)
	if len(fields) != 2 {
		return "", "", false
	}
	return parseDiffPath(fields[0]), parseDiffPath(fields[1]), true
}
