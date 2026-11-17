package hmi

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"
)

// SourceEntity distinguishes the document namespace of a navigation target.
type SourceEntity string

const (
	SourceNode SourceEntity = "node"
	SourceLink SourceEntity = "link"
)

// SourceTarget identifies an authored device or pipe. IDs can be shared by
// nodes and links, so the kind is part of the key.
type SourceTarget struct {
	Kind SourceEntity
	ID   string
}

type sourceSpan struct {
	target         SourceTarget
	start, end, id int
}

// SourceIndex is an immutable index over one exact, validated source snapshot.
// It is safe to pass from a decoder worker to the UI thread. Any source edit
// invalidates its coordinates; a new snapshot requires a new index.
type SourceIndex struct {
	source string
	lines  []int
	spans  []sourceSpan
	ids    map[SourceTarget]int
}

// ParseIndexedSource validates exactly as Parse does, then locates the effective
// top-level nodes and links in the original text (not its formatted serialization).
func ParseIndexedSource(source string) (Project, *SourceIndex, error) {
	if len(source) > MaxDocumentBytes {
		return Project{}, nil, fmt.Errorf("hmi: document too large")
	}
	p, err := Parse([]byte(source))
	if err != nil {
		return Project{}, nil, err
	}
	i := &SourceIndex{source: source, lines: []int{0}, ids: make(map[SourceTarget]int, len(p.Nodes)+len(p.Links))}
	for n := 0; n < len(source); n++ {
		if source[n] == '\n' {
			i.lines = append(i.lines, n+1)
		}
	}
	type region struct{ start, end int }
	arrays := map[SourceEntity]region{}
	d := json.NewDecoder(strings.NewReader(source))
	if _, err := d.Token(); err != nil {
		return Project{}, nil, err
	}
	var raw json.RawMessage
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return Project{}, nil, err
		}
		if err = d.Decode(&raw); err != nil {
			return Project{}, nil, err
		}
		end := int(d.InputOffset())
		r := region{end - len(raw), end}
		k, _ := key.(string)
		switch {
		case strings.EqualFold(k, "nodes"):
			arrays[SourceNode] = r
		case strings.EqualFold(k, "links"):
			arrays[SourceLink] = r
		}
	}
	for _, kind := range []SourceEntity{SourceNode, SourceLink} {
		r := arrays[kind]
		start := len(i.spans)
		if r.start < r.end && source[r.start] == '[' {
			if err := i.indexArray(kind, r.start, r.end); err != nil {
				return Project{}, nil, err
			}
		}
		got := i.spans[start:]
		// JSON may reuse a slice element's earlier fields on duplicate array
		// keys. Never jump to a superseded object or fabricate an ID location.
		i.spans = i.spans[:start]
		for n, span := range got {
			if span.target.ID == "" {
				continue
			}
			if kind == SourceNode && (n >= len(p.Nodes) || span.target.ID != p.Nodes[n].ID) ||
				kind == SourceLink && (n >= len(p.Links) || span.target.ID != p.Links[n].ID) {
				continue
			}
			i.spans = append(i.spans, span)
			i.ids[span.target] = span.id
		}
	}
	slices.SortFunc(i.spans, func(a, b sourceSpan) int { return a.start - b.start })
	return p, i, nil
}

func (i *SourceIndex) indexArray(kind SourceEntity, start, end int) error {
	d := json.NewDecoder(strings.NewReader(i.source[start:end]))
	if _, err := d.Token(); err != nil {
		return err
	}
	var raw json.RawMessage
	for d.More() {
		if err := d.Decode(&raw); err != nil {
			return err
		}
		end := start + int(d.InputOffset())
		begin := end - len(raw)
		id, offset, err := sourceObjectID(i.source[begin:end])
		if err != nil {
			return err
		}
		target := SourceTarget{Kind: kind, ID: id}
		i.spans = append(i.spans, sourceSpan{target: target, start: begin, end: end, id: begin + offset})
	}
	return nil
}

func sourceObjectID(object string) (string, int, error) {
	if object == "null" {
		return "", 0, nil
	}
	d := json.NewDecoder(strings.NewReader(object))
	if _, err := d.Token(); err != nil {
		return "", 0, err
	}
	var raw json.RawMessage
	var id string
	var offset int
	for d.More() {
		key, err := d.Token()
		if err != nil {
			return "", 0, err
		}
		if err := d.Decode(&raw); err != nil {
			return "", 0, err
		}
		// Null leaves a Go string unchanged, matching the schema decoder.
		if k, ok := key.(string); ok && strings.EqualFold(k, "id") && len(raw) != 0 && raw[0] == '"' {
			if err := json.Unmarshal(raw, &id); err != nil {
				return "", 0, err
			}
			offset = int(d.InputOffset()) - len(raw) + 1
		}
	}
	return id, offset, nil
}

// AtPosition finds the entity containing a zero-based line and UTF-8 byte column.
// Object braces belong to the entity; separators and surrounding whitespace do not.
func (i *SourceIndex) AtPosition(line, byteColumn int) (SourceTarget, bool) {
	if i == nil || line < 0 || line >= len(i.lines) || byteColumn < 0 {
		return SourceTarget{}, false
	}
	start, end := i.lines[line], len(i.source)
	if line+1 < len(i.lines) {
		end = i.lines[line+1] - 1
	}
	if byteColumn > end-start {
		return SourceTarget{}, false
	}
	offset := start + byteColumn
	n := sort.Search(len(i.spans), func(n int) bool { return i.spans[n].end > offset })
	if n < len(i.spans) && i.spans[n].start <= offset {
		return i.spans[n].target, true
	}
	return SourceTarget{}, false
}

// IDPosition returns the zero-based line and rune column just inside the
// authored ID string. Escapes remain source text; the ID key is decoded.
func (i *SourceIndex) IDPosition(target SourceTarget) (line, column int, ok bool) {
	if i == nil {
		return 0, 0, false
	}
	offset, ok := i.ids[target]
	if !ok {
		return 0, 0, false
	}
	line = sort.Search(len(i.lines), func(n int) bool { return i.lines[n] > offset }) - 1
	return line, utf8.RuneCountInString(i.source[i.lines[line]:offset]), true
}
