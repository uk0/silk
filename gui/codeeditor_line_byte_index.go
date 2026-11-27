package gui

import (
	"math/bits"
	"slices"
)

const editorLineByteBlockLines = 128

// editorLineByteIndex stores one cumulative-tree node per 128 line bodies.
// Each row contributes its byte length plus one LF. The final virtual LF
// simplifies offsets; lookups beyond the real document still clamp to EOF.
type editorLineByteIndex struct {
	blocks []int
	rows   int
}

func (s *editorLineByteIndex) reset(lines []string) {
	if len(lines) == 0 {
		s.blocks, s.rows = nil, 0
		return
	}
	n := (len(lines)-1)/editorLineByteBlockLines + 2 // slot 0 is empty
	s.blocks = slices.Grow(s.blocks[:0], n)[:n]
	clear(s.blocks)
	s.rows = len(lines)
	for row, line := range lines {
		s.blocks[row/editorLineByteBlockLines+1] += len(line) + 1
	}
	for i := 1; i < n; i++ {
		if parent := i + (i & -i); parent < n {
			s.blocks[parent] += s.blocks[i]
		}
	}
}

func (s *editorLineByteIndex) lineStart(lines []string, row int) int {
	block := row / editorLineByteBlockLines
	start := 0
	for i := block; i > 0; i -= i & -i {
		start += s.blocks[i]
	}
	for _, line := range lines[block*editorLineByteBlockLines : row] {
		start += len(line) + 1
	}
	return start
}

func (s *editorLineByteIndex) applyRowByteDelta(row, delta int) {
	for i := row/editorLineByteBlockLines + 1; i < len(s.blocks); i += i & -i {
		s.blocks[i] += delta
	}
}

func (s *editorLineByteIndex) cursorAtOffset(lines []string, offset int) cursorPos {
	if len(lines) == 0 {
		return cursorPos{}
	}
	offset = max(0, offset)
	last := len(lines) - 1
	block, start := 0, 0
	// Find complete blocks before the offset using tree binary lifting.
	for step := 1 << (bits.Len(uint(len(s.blocks)-1)) - 1); step > 0; step >>= 1 {
		next := block + step
		if next < len(s.blocks) && s.blocks[next] <= offset-start {
			block, start = next, start+s.blocks[next]
		}
	}
	if block == len(s.blocks)-1 {
		return cursorPos{last, runeColumnAtByte(lines[last], len(lines[last]))}
	}
	offset -= start
	for row := block * editorLineByteBlockLines; ; row++ {
		line := lines[row]
		if offset <= len(line) || row == last {
			return cursorPos{row, runeColumnAtByte(line, min(offset, len(line)))}
		}
		offset -= len(line) + 1
	}
}
