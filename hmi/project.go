// Package hmi describes portable, inert 2D industrial process documents.
package hmi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/uk0/silk/point"
)

const Version = 1
const MaxDocumentBytes = 1 << 20

// Point names, categories (DI/AI/BI), actual types, and engineering units are independent.
type Point struct {
	point.Definition
	Initial any `json:"initial,omitempty"`
	StaleMS int `json:"stale_ms,omitempty"`
}
type Source struct {
	ID           string            `json:"id"`
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         string            `json:"body,omitempty"`
	IntervalMS   int               `json:"interval_ms"`
	TimeoutMS    int               `json:"timeout_ms"`
	Enabled      bool              `json:"enabled"`
	RepeatWrites bool              `json:"repeat_writes,omitempty"`
	Mappings     []point.Mapping   `json:"mappings"`
}
type ColorRule struct {
	When  string `json:"when"`
	Color string `json:"color"`
	Text  string `json:"text,omitempty"`
}
type PropertyBinding struct {
	Tag      string       `json:"tag"`
	Property string       `json:"property"`
	Steps    []point.Step `json:"steps,omitempty"`
}
type Node struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Label    string            `json:"label"`
	X        float64           `json:"x"`
	Y        float64           `json:"y"`
	Width    float64           `json:"width"`
	Height   float64           `json:"height"`
	Tag      string            `json:"tag,omitempty"`
	Unit     string            `json:"unit,omitempty"`
	Steps    []point.Step      `json:"steps,omitempty"`
	Colors   []ColorRule       `json:"colors,omitempty"`
	Bindings []PropertyBinding `json:"bindings,omitempty"`
}
type XY struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}
type Endpoint struct {
	Node string `json:"node"`
	Port string `json:"port"`
}
type Link struct {
	ID    string   `json:"id"`
	From  Endpoint `json:"from"`
	To    Endpoint `json:"to"`
	Tag   string   `json:"tag,omitempty"`
	Bends []XY     `json:"bends,omitempty"`
}
type Project struct {
	Version       int      `json:"version"`
	Title         string   `json:"title"`
	Width         float64  `json:"width"`
	Height        float64  `json:"height"`
	Points        []Point  `json:"points"`
	Sources       []Source `json:"sources,omitempty"`
	Nodes         []Node   `json:"nodes"`
	Links         []Link   `json:"links,omitempty"`
	RetentionDays int      `json:"retention_days,omitempty"`
}

func validID(s string) bool {
	if s == "" || len(s) > 256 || strings.TrimSpace(s) != s {
		return false
	}
	for _, r := range s {
		if r < 32 {
			return false
		}
	}
	return true
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
func validHeader(k, value string) bool {
	if k == "" || len(k) > 256 || len(value) > 8192 {
		return false
	}
	for _, c := range k {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("!#$%&'*+-.^_`|~", c)) {
			return false
		}
	}
	for _, c := range value {
		if c == 127 || c < 32 && c != '\t' {
			return false
		}
	}
	return true
}
func validPort(s string) bool { return s == "north" || s == "south" || s == "west" || s == "east" }
func ValidKind(s string) bool {
	switch s {
	case "reactor", "exchanger", "separator", "pump", "valve", "tank", "value", "indicator", "text":
		return true
	}
	return false
}
func (p Project) Validate() error {
	bad := func(s string) error { return fmt.Errorf("hmi: %s", s) }
	if p.Version != Version || !finite(p.Width) || !finite(p.Height) || p.Width < 100 || p.Width > 10000 || p.Height < 100 || p.Height > 10000 || len(p.Points) > 512 || len(p.Sources) > 64 || len(p.Nodes) > 512 || len(p.Links) > 1024 || p.RetentionDays < 0 || p.RetentionDays > 36500 {
		return bad("invalid document version, bounds or budget")
	}
	pts := map[string]point.Kind{}
	for _, d := range p.Points {
		if e := d.Definition.Validate(); e != nil {
			return e
		}
		if !validID(d.Name) || pts[d.Name] != "" || d.StaleMS < 0 || d.StaleMS > 86400000 {
			return bad("invalid or duplicate point")
		}
		pts[d.Name] = d.Kind
		if d.Initial != nil {
			if _, e := point.Convert(d.Initial, d.Kind); e != nil {
				return e
			}
		}
	}
	sources := map[string]bool{}
	writers := map[string]bool{}
	for _, s := range p.Sources {
		u, e := url.Parse(s.URL)
		if !validID(s.ID) || sources[s.ID] || e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || s.IntervalMS < 100 || s.IntervalMS > 3600000 || s.TimeoutMS < 1 || s.TimeoutMS > 300000 || len(s.Body) > 65536 || len(s.Mappings) < 1 || len(s.Mappings) > 128 {
			return bad("invalid source configuration")
		}
		sources[s.ID] = true
		method := strings.ToUpper(s.Method)
		switch method {
		case "GET", "HEAD":
		case "POST", "PUT", "PATCH", "DELETE":
			if s.Enabled && !s.RepeatWrites {
				return bad("repeating write method requires explicit consent")
			}
		default:
			return bad("invalid HTTP method")
		}
		if len(s.Headers) > 128 {
			return bad("too many HTTP headers")
		}
		for k, v := range s.Headers {
			if !validHeader(k, v) || http.CanonicalHeaderKey(k) == "Authorization" || http.CanonicalHeaderKey(k) == "Cookie" {
				return bad("credentials must be injected at runtime; invalid header")
			}
		}
		mapped := map[string]bool{}
		for _, m := range s.Mappings {
			if mapped[m.Name] {
				return bad("duplicate source mapping")
			}
			mapped[m.Name] = true
			if pts[m.Name] == "" || pts[m.Name] != m.Kind || len(m.Pointer) > 4096 || m.Pointer != "" && !strings.HasPrefix(m.Pointer, "/") {
				return bad("source target/type/pointer mismatch")
			}
			if s.Enabled && writers[m.Name] {
				return bad("multiple enabled sources write the same point")
			}
			if s.Enabled {
				writers[m.Name] = true
			}
			if _, e := point.NewPipeline(m.Steps); e != nil {
				return e
			}
		}
	}
	nodes := map[string]bool{}
	for _, n := range p.Nodes {
		if !validID(n.ID) || nodes[n.ID] || !ValidKind(n.Kind) || !finite(n.X) || !finite(n.Y) || !finite(n.Width) || !finite(n.Height) || n.X < 0 || n.Y < 0 || n.Width < 16 || n.Height < 16 || n.X+n.Width > p.Width || n.Y+n.Height > p.Height || n.Tag != "" && pts[n.Tag] == "" || len(n.Label) > 256 || len(n.Colors) > 32 || len(n.Bindings) > 8 {
			return bad("invalid node, bounds or point reference")
		}
		nodes[n.ID] = true
		if _, e := point.NewPipeline(n.Steps); e != nil {
			return e
		}
		for _, r := range n.Colors {
			if _, e := point.Compile(r.When); e != nil {
				return e
			}
			if len(r.Color) != 7 || r.Color[0] != '#' {
				return bad("color must be #RRGGBB")
			}
			for _, c := range r.Color[1:] {
				if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
					return bad("invalid color")
				}
			}
		}
		seen := map[string]bool{}
		for _, b := range n.Bindings {
			if pts[b.Tag] == "" || seen[b.Property] {
				return bad("invalid/duplicate property binding")
			}
			seen[b.Property] = true
			switch b.Property {
			case "visible", "enabled", "x", "y", "rotation":
			default:
				return bad("unknown visual property")
			}
			if _, e := point.NewPipeline(b.Steps); e != nil {
				return e
			}
		}
	}
	links := map[string]bool{}
	for _, l := range p.Links {
		if !validID(l.ID) || links[l.ID] || !nodes[l.From.Node] || !nodes[l.To.Node] || l.From.Node == l.To.Node || !validPort(l.From.Port) || !validPort(l.To.Port) || l.Tag != "" && pts[l.Tag] == "" || len(l.Bends) > 32 {
			return bad("invalid link")
		}
		links[l.ID] = true
		for _, b := range l.Bends {
			if !finite(b.X) || !finite(b.Y) || b.X < 0 || b.Y < 0 || b.X > p.Width || b.Y > p.Height {
				return bad("invalid link bend")
			}
		}
	}
	return nil
}
func Parse(data []byte) (Project, error) {
	var p Project
	if len(data) > MaxDocumentBytes {
		return p, fmt.Errorf("hmi: document too large")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	d.DisallowUnknownFields()
	if e := d.Decode(&p); e != nil {
		return p, e
	}
	var more any
	if d.Decode(&more) != io.EOF {
		return p, fmt.Errorf("hmi: trailing document data")
	}
	return p, p.Validate()
}
func (p Project) JSON() ([]byte, error) {
	if e := p.Validate(); e != nil {
		return nil, e
	}
	data, e := json.MarshalIndent(p, "", "  ")
	if len(data) > MaxDocumentBytes {
		return nil, fmt.Errorf("hmi: document too large")
	}
	return data, e
}

// Save writes atomically in the destination directory; callers own overwrite consent.
func Save(path string, p Project) error {
	b, e := p.JSON()
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".silkhmi-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func Load(path string) (Project, error) {
	f, e := os.Open(path)
	if e != nil {
		return Project{}, e
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, MaxDocumentBytes+1))
	if e != nil {
		return Project{}, e
	}
	return Parse(b)
}
