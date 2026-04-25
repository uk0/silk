package gui

import (
	"bytes"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// GitLineStatus indicates the git modification state of a single line.
type GitLineStatus int

const (
	GitUnchanged GitLineStatus = iota
	GitAdded                   // green bar — new line not in HEAD
	GitModified                // blue bar — changed line
	GitDeleted                 // red triangle — line(s) deleted after this position
)

// GitDiff runs "git diff" on the given file and returns per-line status.
// Lines are 1-based (matching editor display). Returns an empty map if
// git is not available, the file is not tracked, or parsing fails.
func GitDiff(filePath string) map[int]GitLineStatus {
	result := make(map[int]GitLineStatus)
	if filePath == "" {
		return result
	}

	cmd := exec.Command("git", "diff", "--unified=0", "--no-color", filePath)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Start(); err != nil {
		return result
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		if err != nil {
			// git not installed, file not tracked, or no changes
			return result
		}
	case <-time.After(2 * time.Second):
		cmd.Process.Kill()
		return result
	}

	lines := strings.Split(out.String(), "\n")
	for _, line := range lines {
		if !strings.HasPrefix(line, "@@") {
			continue
		}
		// Parse hunk header: @@ -a,b +c,d @@
		// or @@ -a +c,d @@  or  @@ -a,b +c @@  etc.
		_, oldCount, newStart, newCount := parseHunkHeader(line)

		if oldCount == 0 && newCount > 0 {
			// Pure addition: lines newStart..newStart+newCount-1 are added
			for i := 0; i < newCount; i++ {
				result[newStart+i] = GitAdded
			}
		} else if newCount == 0 && oldCount > 0 {
			// Pure deletion: mark the line after deletion point
			if newStart > 0 {
				result[newStart] = GitDeleted
			} else {
				result[1] = GitDeleted
			}
		} else if newCount > 0 {
			// Modification: lines were changed
			for i := 0; i < newCount; i++ {
				result[newStart+i] = GitModified
			}
		}
	}
	return result
}

// parseHunkHeader extracts start/count from a unified diff hunk header.
// Format: @@ -oldStart,oldCount +newStart,newCount @@
// When count is omitted, it defaults to 1.
func parseHunkHeader(header string) (oldStart, oldCount, newStart, newCount int) {
	// Find the range specifications between @@ markers
	parts := strings.SplitN(header, "@@", 3)
	if len(parts) < 2 {
		return
	}
	rangePart := strings.TrimSpace(parts[1])
	// rangePart looks like: "-a,b +c,d" or "-a +c" etc.
	fields := strings.Fields(rangePart)
	for _, f := range fields {
		if strings.HasPrefix(f, "-") {
			oldStart, oldCount = parseRange(f[1:])
		} else if strings.HasPrefix(f, "+") {
			newStart, newCount = parseRange(f[1:])
		}
	}
	return
}

// parseRange parses "start,count" or "start" (count defaults to 1).
func parseRange(s string) (start, count int) {
	if idx := strings.Index(s, ","); idx >= 0 {
		start, _ = strconv.Atoi(s[:idx])
		count, _ = strconv.Atoi(s[idx+1:])
	} else {
		start, _ = strconv.Atoi(s)
		count = 1
	}
	return
}
