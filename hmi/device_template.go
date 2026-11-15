package hmi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"unicode/utf8"
)

const MaxDeviceLibraryBytes = 4 << 20
const MaxDeviceTemplates = 64

var ErrDeviceLibraryExists = errors.New("hmi: device library destination exists")

// DeviceTemplate is an inert reusable subgraph. Sources and retention policy
// are deliberately forbidden: placing a device never creates another driver.
// Values are caller-owned; do not mutate them concurrently with SDK operations.
type DeviceTemplate struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Project Project `json:"project"`
}

type DeviceLibrary struct {
	Version int              `json:"version"`
	Devices []DeviceTemplate `json:"devices"`
}

type DevicePlacement struct {
	ID          string
	X, Y        float64
	PointPrefix string            // empty means ID + "."; otherwise prepended exactly
	Points      map[string]string // explicit source-to-destination point mapping
}

type DeviceInstance struct {
	Nodes  []string // authored order, ready for GUI selection
	Links  []string
	Points map[string]string
}

func deviceID(s string) bool { return validID(s) && utf8.ValidString(s) }

func (t DeviceTemplate) Validate() error {
	if !deviceID(t.ID) || !deviceID(t.Name) || len(t.Project.Nodes) == 0 || len(t.Project.Sources) != 0 || t.Project.RetentionDays != 0 {
		return fmt.Errorf("hmi: invalid device template metadata, nodes or acquisition policy")
	}
	for _, n := range t.Project.Nodes {
		if !deviceID(n.ID) {
			return fmt.Errorf("hmi: invalid template node ID")
		}
	}
	for _, d := range t.Project.Points {
		if !deviceID(d.Name) {
			return fmt.Errorf("hmi: invalid template point name")
		}
	}
	for _, l := range t.Project.Links {
		if !deviceID(l.ID) {
			return fmt.Errorf("hmi: invalid template pipe ID")
		}
	}
	_, err := t.Project.JSON()
	return err
}

// CaptureDeviceTemplate copies selected nodes, internal pipes (including bends)
// and all referenced point definitions. Geometry is normalized to its bounding
// origin. Acquisition sources and live samples are never captured.
func CaptureDeviceTemplate(p Project, ids []string, id, name string) (DeviceTemplate, error) {
	m, err := editSelection(p, ids, 1)
	if err != nil {
		return DeviceTemplate{}, err
	}
	copy, err := CloneProject(p)
	if err != nil {
		return DeviceTemplate{}, err
	}
	b := selectionBounds(copy, m, true)
	content := Project{Version: Version, Title: name, Width: max(100, b.Width), Height: max(100, b.Height)}
	points := map[string]bool{}
	for _, n := range copy.Nodes {
		if m[n.ID] {
			n.X, n.Y = n.X-b.X, n.Y-b.Y
			content.Nodes = append(content.Nodes, n)
			points[n.Tag] = true
			for _, bind := range n.Bindings {
				points[bind.Tag] = true
			}
		}
	}
	for _, l := range copy.Links {
		if m[l.From.Node] && m[l.To.Node] {
			for i := range l.Bends {
				l.Bends[i].X -= b.X
				l.Bends[i].Y -= b.Y
			}
			content.Links = append(content.Links, l)
			points[l.Tag] = true
		}
	}
	for _, d := range copy.Points {
		if points[d.Name] {
			content.Points = append(content.Points, d)
		}
	}
	// Translated sums can round differently from the original bounding sums.
	for _, n := range content.Nodes {
		content.Width, content.Height = max(content.Width, n.X+n.Width), max(content.Height, n.Y+n.Height)
	}
	for _, l := range content.Links {
		for _, p := range l.Bends {
			content.Width, content.Height = max(content.Width, p.X), max(content.Height, p.Y)
		}
	}
	t := DeviceTemplate{ID: id, Name: name, Project: content}
	if err := t.Validate(); err != nil {
		return DeviceTemplate{}, err
	}
	return t, nil
}

// PlaceDeviceTemplate returns a complete independent candidate or an error with
// no mutation. Default point names are independent per instance. Sharing an
// existing point requires an explicit mapping and matching type/category/unit;
// its host initial value, timeout and acquisition owner are kept unchanged.
func PlaceDeviceTemplate(p Project, t DeviceTemplate, o DevicePlacement) (Project, DeviceInstance, error) {
	fail := func(err error) (Project, DeviceInstance, error) { return Project{}, DeviceInstance{}, err }
	if err := t.Validate(); err != nil {
		return fail(err)
	}
	if !deviceID(o.ID) || !finite(o.X) || !finite(o.Y) || o.X < 0 || o.Y < 0 {
		return fail(fmt.Errorf("hmi: invalid device instance ID or position"))
	}
	q, err := CloneProject(p)
	if err != nil {
		return fail(err)
	}
	source, err := CloneProject(t.Project)
	if err != nil {
		return fail(err)
	}
	selected := map[string]bool{}
	for _, n := range source.Nodes {
		selected[n.ID] = true
	}
	b := selectionBounds(source, selected, true)
	if o.X+b.Width > q.Width || o.Y+b.Height > q.Height {
		return fail(fmt.Errorf("hmi: complete device subgraph does not fit the document"))
	}
	dx, dy := o.X-b.X, o.Y-b.Y
	prefix := o.PointPrefix
	if prefix == "" {
		prefix = o.ID + "."
	}
	pointDefs := map[string]Point{}
	for _, d := range q.Points {
		pointDefs[d.Name] = d
	}
	known := map[string]bool{}
	for _, d := range source.Points {
		known[d.Name] = true
	}
	for name := range o.Points {
		if !known[name] {
			return fail(fmt.Errorf("hmi: unknown template point mapping %q", name))
		}
	}
	result := DeviceInstance{Points: map[string]string{}}
	usedTargets := map[string]bool{}
	for _, d := range source.Points {
		name, explicit := o.Points[d.Name]
		if !explicit {
			name = prefix + d.Name
		}
		if !deviceID(name) || usedTargets[name] {
			return fail(fmt.Errorf("hmi: invalid or aliased destination point %q", name))
		}
		usedTargets[name] = true
		if host, exists := pointDefs[name]; exists {
			if !explicit || host.Kind != d.Kind || host.Unit != d.Unit || host.Category != d.Category {
				return fail(fmt.Errorf("hmi: destination point %q collides or has incompatible type/category/unit", name))
			}
		} else {
			d.Name = name
			q.Points = append(q.Points, d)
			pointDefs[name] = d
		}
	}
	for _, d := range source.Points {
		name, ok := o.Points[d.Name]
		if !ok {
			name = prefix + d.Name
		}
		result.Points[d.Name] = name
	}
	nodes, links := map[string]bool{}, map[string]bool{}
	for _, n := range q.Nodes {
		nodes[n.ID] = true
	}
	for _, l := range q.Links {
		links[l.ID] = true
	}
	remap := map[string]string{}
	for _, n := range source.Nodes {
		id := o.ID + "." + n.ID
		if !deviceID(id) || nodes[id] {
			return fail(fmt.Errorf("hmi: invalid or colliding device node %q", id))
		}
		remap[n.ID] = id
		n.ID, n.X, n.Y = id, n.X+dx, n.Y+dy
		n.Tag = result.Points[n.Tag]
		for i := range n.Bindings {
			n.Bindings[i].Tag = result.Points[n.Bindings[i].Tag]
		}
		q.Nodes = append(q.Nodes, n)
		result.Nodes = append(result.Nodes, id)
	}
	for _, l := range source.Links {
		id := o.ID + "." + l.ID
		if !deviceID(id) || links[id] {
			return fail(fmt.Errorf("hmi: invalid or colliding device pipe %q", id))
		}
		l.ID, l.From.Node, l.To.Node, l.Tag = id, remap[l.From.Node], remap[l.To.Node], result.Points[l.Tag]
		for i := range l.Bends {
			l.Bends[i].X += dx
			l.Bends[i].Y += dy
		}
		q.Links = append(q.Links, l)
		result.Links = append(result.Links, id)
	}
	q, err = finishEdit(q)
	if err != nil {
		return fail(err)
	}
	return q, result, nil
}

func (l DeviceLibrary) Validate() error {
	if l.Version != 1 || len(l.Devices) > MaxDeviceTemplates {
		return fmt.Errorf("hmi: invalid library version or device budget")
	}
	seen := map[string]bool{}
	total := 0
	for _, t := range l.Devices {
		if seen[t.ID] {
			return fmt.Errorf("hmi: duplicate library device %q", t.ID)
		}
		seen[t.ID] = true
		if err := t.Validate(); err != nil {
			return err
		}
		b, err := t.Project.JSON()
		if err != nil {
			return err
		}
		total += len(b)
		if total > MaxDeviceLibraryBytes {
			return fmt.Errorf("hmi: device library byte budget exceeded")
		}
	}
	return nil
}
func (l DeviceLibrary) JSON() ([]byte, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	b, err := json.MarshalIndent(l, "", "  ")
	if len(b) > MaxDeviceLibraryBytes {
		return nil, fmt.Errorf("hmi: device library byte budget exceeded")
	}
	return b, err
}
func ParseDeviceLibrary(b []byte) (DeviceLibrary, error) {
	var l DeviceLibrary
	if len(b) > MaxDeviceLibraryBytes {
		return l, fmt.Errorf("hmi: device library byte budget exceeded")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	d.DisallowUnknownFields()
	if err := d.Decode(&l); err != nil {
		return l, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return l, fmt.Errorf("hmi: trailing device library data")
	}
	if _, err := l.JSON(); err != nil {
		return DeviceLibrary{}, err
	}
	return l, nil
}
func CloneDeviceLibrary(l DeviceLibrary) (DeviceLibrary, error) {
	if _, err := l.JSON(); err != nil {
		return DeviceLibrary{}, err
	}
	result := DeviceLibrary{Version: l.Version}
	for _, t := range l.Devices {
		p, err := CloneProject(t.Project)
		if err != nil {
			return DeviceLibrary{}, err
		}
		t.Project = p
		result.Devices = append(result.Devices, t)
	}
	return result, nil
}

// LoadDeviceLibrary requires an explicit caller-owned worker. No source in a
// file is executed; non-regular files and oversized input are rejected.
func LoadDeviceLibrary(ctx context.Context, path string) (DeviceLibrary, error) {
	if ctx == nil {
		return DeviceLibrary{}, fmt.Errorf("hmi: nil library I/O context")
	}
	if err := ctx.Err(); err != nil {
		return DeviceLibrary{}, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return DeviceLibrary{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxDeviceLibraryBytes {
		return DeviceLibrary{}, fmt.Errorf("hmi: library path must be a bounded regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return DeviceLibrary{}, err
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil {
		return DeviceLibrary{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxDeviceLibraryBytes {
		return DeviceLibrary{}, fmt.Errorf("hmi: library must be a bounded regular file")
	}
	b, err := io.ReadAll(io.LimitReader(f, MaxDeviceLibraryBytes+1))
	if err != nil {
		return DeviceLibrary{}, err
	}
	if err = ctx.Err(); err != nil {
		return DeviceLibrary{}, err
	}
	return ParseDeviceLibrary(b)
}

// SaveDeviceLibrary validates and replaces atomically in the same directory.
// Existing destinations require explicit overwrite consent. Cancellation is
// checked before commit; this is not a filesystem CAS or crash-durability proof.
func SaveDeviceLibrary(ctx context.Context, path string, l DeviceLibrary, overwrite bool) error {
	if ctx == nil {
		return fmt.Errorf("hmi: nil library I/O context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	b, err := l.JSON()
	if err != nil {
		return err
	}
	info, statErr := os.Lstat(path)
	if statErr == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("hmi: library destination is not a regular file")
		}
		if !overwrite {
			return ErrDeviceLibraryExists
		}
	}
	if statErr != nil && !os.IsNotExist(statErr) {
		return statErr
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".silkhmilibrary-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if !overwrite {
		// Link is an atomic no-replace commit, unlike Stat followed by Rename.
		if err = os.Link(f.Name(), path); os.IsExist(err) {
			return ErrDeviceLibraryExists
		}
		return err
	}
	return os.Rename(f.Name(), path)
}
