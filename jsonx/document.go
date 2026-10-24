// Package jsonx provides immutable, bounded JSON documents for API tools and
// model/view consumers. Numbers retain their original json.Number precision.
package jsonx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

var (
	ErrPointer  = errors.New("jsonx: invalid JSON pointer")
	ErrNotFound = errors.New("jsonx: value not found")
	ErrType     = errors.New("jsonx: unexpected value type")
	ErrLimit    = errors.New("jsonx: document limit exceeded")
	ErrRange    = errors.New("jsonx: child index out of range")
)

// Options use defaults for zero values. Negative values are invalid. Limits are
// checked before publishing a document; MaxBytes also bounds decoder allocations.
// The standard decoder rejects nesting beyond its own limit before our walk.
type Options struct{ MaxBytes, MaxDepth, MaxNodes int }

const (
	DefaultMaxBytes = 4 << 20
	DefaultMaxDepth = 128
	DefaultMaxNodes = 100000
)

type Kind string

const (
	Invalid Kind = "invalid"
	Object  Kind = "object"
	Array   Kind = "array"
	String  Kind = "string"
	Number  Kind = "number"
	Boolean Kind = "boolean"
	Null    Kind = "null"
)

type entry struct {
	kind     Kind
	scalar   any
	keys     []string
	children []*entry
}

// Document and Node are immutable after Parse and safe for concurrent readers.
// Values/JSON returned to callers are copies, never writable internal storage.
type Document struct {
	root  *entry
	count int
}
type Node struct {
	e            *entry
	pointer, key string
}

func Parse(data []byte) (*Document, error) { return ParseWithOptions(data, Options{}) }
func ParseWithOptions(data []byte, o Options) (*Document, error) {
	if o.MaxBytes < 0 || o.MaxDepth < 0 || o.MaxNodes < 0 {
		return nil, fmt.Errorf("jsonx: limits must be nonnegative")
	}
	if o.MaxBytes == 0 {
		o.MaxBytes = DefaultMaxBytes
	}
	if o.MaxDepth == 0 {
		o.MaxDepth = DefaultMaxDepth
	}
	if o.MaxNodes == 0 {
		o.MaxNodes = DefaultMaxNodes
	}
	if len(data) > o.MaxBytes {
		return nil, fmt.Errorf("%w: bytes", ErrLimit)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("jsonx: invalid UTF-8")
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("jsonx: decode: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("jsonx: expected exactly one JSON value")
	}
	d := &Document{}
	var build func(any, int) (*entry, error)
	build = func(v any, depth int) (*entry, error) {
		d.count++
		if depth > o.MaxDepth || d.count > o.MaxNodes {
			return nil, ErrLimit
		}
		e := &entry{}
		switch v := v.(type) {
		case map[string]any:
			e.kind = Object
			e.keys = make([]string, 0, len(v))
			for k := range v {
				e.keys = append(e.keys, k)
			}
			sort.Strings(e.keys)
			e.children = make([]*entry, 0, len(v))
			for _, k := range e.keys {
				c, err := build(v[k], depth+1)
				if err != nil {
					return nil, err
				}
				e.children = append(e.children, c)
			}
		case []any:
			e.kind = Array
			e.children = make([]*entry, 0, len(v))
			for _, item := range v {
				c, err := build(item, depth+1)
				if err != nil {
					return nil, err
				}
				e.children = append(e.children, c)
			}
		case string:
			e.kind = String
			e.scalar = v
		case json.Number:
			e.kind = Number
			e.scalar = v
		case bool:
			e.kind = Boolean
			e.scalar = v
		case nil:
			e.kind = Null
		default:
			return nil, ErrType
		}
		return e, nil
	}
	var err error
	d.root, err = build(value, 0)
	if err != nil {
		return nil, err
	}
	return d, nil
}
func (d *Document) Root() Node {
	if d == nil {
		return Node{}
	}
	return Node{e: d.root, key: "$"}
}
func (d *Document) NodeCount() int {
	if d == nil {
		return 0
	}
	return d.count
}
func (n Node) Valid() bool { return n.e != nil }
func (n Node) Kind() Kind {
	if n.e == nil {
		return Invalid
	}
	return n.e.kind
}
func (n Node) Pointer() string { return n.pointer }
func (n Node) Key() string     { return n.key }
func (n Node) Len() int {
	if n.e == nil {
		return 0
	}
	return len(n.e.children)
}

// JoinPointer appends an object key or canonical array index to a valid base.
// It escapes tokens, not the whole pointer. Empty keys are represented by '/'.
func JoinPointer(base, token string) string {
	return base + "/" + strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}

// SplitPointer accepts RFC 6901 string syntax, not URI fragments or JSONPath.
func SplitPointer(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}
	if pointer[0] != '/' || !utf8.ValidString(pointer) {
		return nil, ErrPointer
	}
	parts := strings.Split(pointer[1:], "/")
	for j, part := range parts {
		var b strings.Builder
		for i := 0; i < len(part); i++ {
			if part[i] != '~' {
				b.WriteByte(part[i])
				continue
			}
			i++
			if i == len(part) {
				return nil, ErrPointer
			}
			switch part[i] {
			case '0':
				b.WriteByte('~')
			case '1':
				b.WriteByte('/')
			default:
				return nil, ErrPointer
			}
		}
		parts[j] = b.String()
	}
	return parts, nil
}
func (n Node) ChildIndex(key string) (int, error) {
	switch n.Kind() {
	case Object:
		i := sort.SearchStrings(n.e.keys, key)
		if i == len(n.e.keys) || n.e.keys[i] != key {
			return 0, ErrNotFound
		}
		return i, nil
	case Array:
		if key == "" || (len(key) > 1 && key[0] == '0') {
			return 0, ErrPointer
		}
		for _, c := range key {
			if c < '0' || c > '9' {
				return 0, ErrPointer
			}
		}
		i, err := strconv.ParseUint(key, 10, 64)
		if err != nil || i >= uint64(n.Len()) {
			return 0, ErrNotFound
		}
		return int(i), nil
	default:
		return 0, ErrNotFound
	}
}
func (n Node) Child(index int) (Node, error) {
	if index < 0 || index >= n.Len() {
		return Node{}, ErrRange
	}
	key := strconv.Itoa(index)
	if n.Kind() == Object {
		key = n.e.keys[index]
	}
	return Node{e: n.e.children[index], key: key, pointer: JoinPointer(n.pointer, key)}, nil
}

// Children returns at most limit direct children, with deterministic sorted
// object keys. It does not recursively materialize descendants.
func (n Node) Children(offset, limit int) ([]Node, error) {
	if offset < 0 || offset > n.Len() || limit < 0 {
		return nil, ErrRange
	}
	count := min(limit, n.Len()-offset)
	out := make([]Node, 0, count)
	for i := 0; i < count; i++ {
		c, _ := n.Child(offset + i)
		out = append(out, c)
	}
	return out, nil
}
func (d *Document) Lookup(pointer string) (Node, error) {
	tokens, err := SplitPointer(pointer)
	if err != nil {
		return Node{}, err
	}
	n := d.Root()
	if !n.Valid() {
		return Node{}, ErrNotFound
	}
	for _, token := range tokens {
		i, err := n.ChildIndex(token)
		if err != nil {
			return Node{}, err
		}
		n, _ = n.Child(i)
	}
	return n, nil
}
func (n Node) Value() any {
	if !n.Valid() {
		return nil
	}
	switch n.Kind() {
	case Object:
		out := make(map[string]any, n.Len())
		for i, k := range n.e.keys {
			out[k] = (Node{e: n.e.children[i]}).Value()
		}
		return out
	case Array:
		out := make([]any, n.Len())
		for i, c := range n.e.children {
			out[i] = (Node{e: c}).Value()
		}
		return out
	default:
		return n.e.scalar
	}
}
func (n Node) JSON() ([]byte, error) {
	if !n.Valid() {
		return nil, ErrNotFound
	}
	return json.Marshal(n.Value())
}
func (n Node) Str() (string, error) {
	if n.Kind() != String {
		return "", ErrType
	}
	return n.e.scalar.(string), nil
}
func (n Node) Int() (int64, error) {
	if n.Kind() != Number {
		return 0, ErrType
	}
	return n.e.scalar.(json.Number).Int64()
}
func (n Node) Float() (float64, error) {
	if n.Kind() != Number {
		return 0, ErrType
	}
	v, e := n.e.scalar.(json.Number).Float64()
	if e != nil {
		return 0, e
	}
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, ErrType
	}
	return v, nil
}
func (n Node) Bool() (bool, error) {
	if n.Kind() != Boolean {
		return false, ErrType
	}
	return n.e.scalar.(bool), nil
}

// Preview is bounded display text, not a serialization API. Containers are
// summarized without walking their contents; control characters are escaped.
func (n Node) Preview(maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	maxRunes = min(maxRunes, 4096)
	var text string
	switch n.Kind() {
	case Object:
		text = fmt.Sprintf("{%d fields}", n.Len())
	case Array:
		text = fmt.Sprintf("[%d items]", n.Len())
	case String:
		// Bound before quoting too, so rendering never serializes a huge string.
		text = strconv.Quote(previewPrefix(n.e.scalar.(string), maxRunes))
	case Number:
		text = string(n.e.scalar.(json.Number))
	case Boolean:
		text = strconv.FormatBool(n.e.scalar.(bool))
	case Null:
		text = "null"
	default:
		return ""
	}
	return previewPrefix(text, maxRunes)
}

func previewPrefix(text string, limit int) string {
	count, previous := 0, 0
	for i := range text {
		if count == limit {
			return text[:previous] + "…"
		}
		previous = i
		count++
	}
	return text
}

func (d *Document) Str(pointer string) (string, error) {
	n, err := d.Lookup(pointer)
	if err != nil {
		return "", err
	}
	return n.Str()
}

func (d *Document) Int(pointer string) (int64, error) {
	n, err := d.Lookup(pointer)
	if err != nil {
		return 0, err
	}
	return n.Int()
}

func (d *Document) Float(pointer string) (float64, error) {
	n, err := d.Lookup(pointer)
	if err != nil {
		return 0, err
	}
	return n.Float()
}

func (d *Document) Bool(pointer string) (bool, error) {
	n, err := d.Lookup(pointer)
	if err != nil {
		return false, err
	}
	return n.Bool()
}
