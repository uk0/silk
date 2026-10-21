package macro

import (
	"github.com/uk0/silk/diagnostic"
	"go/scanner"
)

type originChunk struct {
	start, end, source int
	direct             bool
}
type originMap []originChunk

func (m *originMap) add(start, size, source int, direct bool) {
	if size > 0 {
		*m = append(*m, originChunk{start, start + size, source, direct})
	}
}
func (m originMap) errorLocation(filename string, src []byte, err error) diagnostic.Location {
	if list, ok := err.(scanner.ErrorList); ok && len(list) > 0 {
		offset := list[0].Pos.Offset
		for i := len(m) - 1; i >= 0; i-- {
			c := m[i]
			if offset >= c.start && offset <= c.end {
				p := c.source
				if c.direct {
					p += min(offset-c.start, c.end-c.start)
				}
				return sourceLocation(filename, src, p)
			}
		}
	}
	return diagnostic.Location{File: filename}
}
