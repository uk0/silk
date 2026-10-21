package core

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/uk0/silk/diagnostic"
)

// LoadTDoc1 reads at most lineLimit nodes (0 means the complete document).
// Plain documents end at EOF or one blank line. Pass a retained bufio.Reader
// when decoding successive documents; an internal read-ahead buffer is otherwise
// discarded. A full gzip load validates exactly one member, including its CRC,
// and leaves the next member in that retained reader. Limited gzip loads are
// prefix inspections only: they neither validate nor expose the unread suffix.
func LoadTDoc1(r io.Reader, lineLimit uint64) (*TDoc, error) {
	if r == nil {
		return nil, diagnostic.New(diagnostic.DocumentRead, "nil TDoc reader", errors.New("nil reader"))
	}
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	header, err := br.Peek(2)
	if len(header) == 0 && err != nil {
		return nil, diagnostic.New(diagnostic.DocumentRead, "cannot read TDoc header", err)
	}
	if len(header) == 2 && header[0] == 0x1f && header[1] == 0x8b {
		zr, err := gzip.NewReader(br)
		if err != nil {
			return nil, diagnostic.New(diagnostic.DocumentCompressed, "cannot read gzip header", err)
		}
		zr.Multistream(false)
		doc, err := parseTDoc(bufio.NewReader(zr), lineLimit)
		if err == nil && lineLimit == 0 {
			// The plain parser may stop at a blank-line delimiter. Finish this gzip
			// member to validate its trailer without retaining a second full copy.
			_, err = io.Copy(io.Discard, zr)
		}
		err = errors.Join(err, zr.Close())
		if err != nil {
			if diagnostic.CodeOf(err) == diagnostic.DocumentSyntax {
				return nil, err
			}
			return nil, diagnostic.New(diagnostic.DocumentCompressed, "cannot decode gzip TDoc member", err)
		}
		return doc, nil
	}
	// Peek may return one valid byte plus a read error. Preserve that error for
	// the parser instead of losing it through Peek's readErr consumption.
	if err != nil && !errors.Is(err, io.EOF) {
		// A one-byte header cannot contain a complete terminated prefix.
		return nil, diagnostic.New(diagnostic.DocumentRead, "cannot read TDoc header", err)
	}
	return parseTDoc(br, lineLimit)
}

func tdocFileError(err error, path string) error {
	var detail *diagnostic.Error
	if errors.As(err, &detail) {
		loc := detail.Location
		loc.File = path
		return detail.At(loc)
	}
	return diagnostic.New(diagnostic.DocumentRead, "cannot open TDoc file", err).At(diagnostic.Location{File: path})
}

type tdocPosition struct{ line, column, offset int }
type tdocReader struct {
	r             *bufio.Reader
	pos, previous tdocPosition
}

func (p *tdocReader) location(pos tdocPosition) diagnostic.Location {
	return diagnostic.Location{Line: pos.line, Column: pos.column, Offset: pos.offset}
}
func (p *tdocReader) syntax(message string, cause error) error {
	return diagnostic.New(diagnostic.DocumentSyntax, message, cause).At(p.location(p.previous))
}
func (p *tdocReader) readError(err error) error {
	return diagnostic.New(diagnostic.DocumentRead, "cannot read TDoc stream", err).At(p.location(p.pos))
}
func (p *tdocReader) read() (rune, error) {
	p.previous = p.pos
	ch, size, err := p.r.ReadRune()
	if err != nil {
		return 0, err
	}
	if ch == 0 {
		return 0, p.syntax("NUL is not valid TDoc text", nil)
	}
	if ch == utf8.RuneError && size == 1 {
		return 0, p.syntax("invalid UTF-8 in TDoc text", nil)
	}
	p.pos.offset += size
	if ch == '\n' || ch == '\r' {
		p.pos.line++
		p.pos.column = 1
	} else {
		p.pos.column += size
	}
	return ch, nil
}
func (p *tdocReader) unread() error {
	if err := p.r.UnreadRune(); err != nil {
		return err
	}
	p.pos = p.previous
	return nil
}
func (p *tdocReader) spaces() (rune, error) {
	for {
		ch, err := p.read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, nil
			}
			return 0, err
		}
		if ch != ' ' && ch != '\t' {
			return ch, p.unread()
		}
	}
}
func (p *tdocReader) key() (string, error) {
	var out strings.Builder
	for {
		ch, err := p.read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out.String(), nil
			}
			return "", err
		}
		if ch == '\\' {
			ch, err = p.read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return "", p.syntax("unfinished TDoc key escape", io.ErrUnexpectedEOF)
				}
				return "", err
			}
			switch ch {
			case 'n':
				ch = '\n'
			case 'r':
				ch = '\r'
			case 't':
				ch = '\t'
			case 's':
				ch = ' '
			case '\n', '\r':
				return out.String(), p.unread()
			}
			if ch < 32 && ch != '\t' && ch != '\n' && ch != '\r' {
				return "", p.syntax("illegal control character in TDoc key escape", nil)
			}
			// Unknown escapes retain the historical interpretation: escaped character.
			out.WriteRune(ch)
			continue
		}
		if ch == '=' || ch <= 32 {
			return out.String(), p.unread()
		}
		out.WriteRune(ch)
	}
}
func (p *tdocReader) value() (string, rune, error) {
	var out strings.Builder
	for {
		ch, err := p.read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out.String(), 0, nil
			}
			return "", 0, err
		}
		if ch == '\n' || ch == '\r' {
			return out.String(), ch, p.unread()
		}
		out.WriteRune(ch)
	}
}
func (p *tdocReader) entry() (string, string, rune, error) {
	key, err := p.key()
	if err != nil {
		return "", "", 0, err
	}
	ch, err := p.spaces()
	if err != nil {
		return "", "", 0, err
	}
	if ch != '=' {
		return key, "", ch, nil
	}
	if _, err = p.read(); err != nil {
		return "", "", 0, err
	}
	ch, err = p.spaces()
	if err != nil {
		return "", "", 0, err
	}
	if ch == 0 || ch == '\r' || ch == '\n' {
		return key, "", ch, nil
	}
	if ch < 32 {
		return "", "", 0, p.syntax("illegal space/control character in TDoc", nil)
	}
	value, ch, err := p.value()
	return key, value, ch, err
}
func (p *tdocReader) lineEnd(ch rune) error {
	if ch != '\r' && ch != '\n' {
		return nil
	}
	if _, err := p.read(); err != nil {
		return err
	}
	next, err := p.r.Peek(1)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		// Peek consumes a pending error when its buffer is empty. Never hide it,
		// even at a document/line-limit boundary: a retained caller reader could
		// otherwise report EOF on the next load and lose the actual I/O failure.
		return err
	}
	if ch == '\r' && next[0] == '\n' || ch == '\n' && next[0] == '\r' {
		if _, err = p.read(); err != nil {
			return err
		}
		p.pos.line-- // paired newline is one line
	}
	return nil
}

func parseTDoc(br *bufio.Reader, lineLimit uint64) (doc *TDoc, err error) {
	p := &tdocReader{r: br, pos: tdocPosition{line: 1, column: 1}}
	defer func() {
		if err != nil {
			doc = nil
			var known *diagnostic.Error
			if !errors.As(err, &known) {
				err = p.readError(err)
			}
		}
	}()
	ch, err := p.read()
	if err != nil {
		return nil, err
	}
	if ch <= 32 {
		return nil, p.syntax("First line must not indented.", nil)
	}
	if err = p.unread(); err != nil {
		return nil, err
	}
	key, value, end, err := p.entry()
	if err != nil {
		return nil, err
	}
	if err = p.lineEnd(end); err != nil {
		return nil, err
	}
	root := &TDoc{key: key, val: value}
	const maxIndent = 128
	var tracks [maxIndent + 1]*TDoc
	tracks[0] = root
	indent := 0
	for count := uint64(1); lineLimit == 0 || count < lineLimit; count++ {
		lineStart := p.pos
		depth := 0
		for {
			ch, err = p.read()
			if err != nil {
				if errors.Is(err, io.EOF) {
					return root, nil
				}
				return nil, err
			}
			if ch == '\t' {
				depth++
				continue
			}
			if ch == '\r' || ch == '\n' {
				if err = p.unread(); err != nil {
					return nil, err
				}
				if err = p.lineEnd(ch); err != nil {
					return nil, err
				}
				return root, nil
			}
			if ch <= 32 {
				return nil, p.syntax("Illegal indent char detected", nil)
			}
			if err = p.unread(); err != nil {
				return nil, err
			}
			break
		}
		if depth-indent > 1 {
			return nil, diagnostic.New(diagnostic.DocumentSyntax, "Jump indent in line", nil).At(p.location(lineStart))
		}
		if depth >= maxIndent {
			return nil, diagnostic.New(diagnostic.DocumentSyntax, "Indent exceed "+strconv.Itoa(maxIndent), nil).At(p.location(lineStart))
		}
		indent = depth
		parent := tracks[indent]
		if parent == nil {
			return nil, diagnostic.New(diagnostic.DocumentSyntax, "Jump indent in line", nil).At(p.location(lineStart))
		}
		key, value, end, err = p.entry()
		if err != nil {
			return nil, err
		}
		child := &TDoc{key: key, val: value}
		parent.AddChild(child)
		tracks[indent+1] = child
		if err = p.lineEnd(end); err != nil {
			return nil, err
		}
	}
	return root, nil
}
