package core

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// LSPWorkProgress is one server-reported work-done operation, not a count of
// outstanding RPCs or a guarantee that the server has no unreported work.
// Token preserves the distinction between numeric and string progress tokens.
type LSPWorkProgress struct {
	Token          json.RawMessage
	Title, Message string
	Percentage     *uint32
	Cancellable    bool
}

// LSPWorkProgressSnapshot is an owned copy, ordered by operation start time.
// Limited remains true after a capacity/size limit was exceeded: an incomplete
// snapshot must never masquerade as proof that the server is idle.
type LSPWorkProgressSnapshot struct {
	Active          []LSPWorkProgress
	LastCompleted   string
	Limited, Closed bool
}

type lspProgressEntry struct {
	value LSPWorkProgress
	order uint64
}
type lspProgressState struct {
	mu              sync.Mutex
	updates         chan struct{}
	active          map[string]lspProgressEntry
	sequence        uint64
	lastCompleted   string
	limited, closed bool
}

// WorkProgressUpdates opts into a separate, coalesced work-done progress lane.
// Call before Initialize and advertise window.workDoneProgress in your own
// capabilities. Each wake means "read WorkProgressSnapshot", not one wire event.
// The single-slot channel closes with the transport. It is safe to leave it
// unread: progress never occupies the diagnostics/notification queue. Partial
// result progress (without begin/report/end kind) still uses Notifications.
// DefaultClientCapabilities intentionally does not opt hosts into this UI policy.
func (c *LSPClient) WorkProgressUpdates() <-chan struct{} {
	p := &c.workProgress
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.updates == nil {
		p.updates = make(chan struct{}, 1)
		if p.closed {
			close(p.updates)
		} else {
			p.active = make(map[string]lspProgressEntry)
		}
	}
	return p.updates
}

func (c *LSPClient) WorkProgressSnapshot() LSPWorkProgressSnapshot {
	p := &c.workProgress
	p.mu.Lock()
	defer p.mu.Unlock()
	entries := make([]lspProgressEntry, 0, len(p.active))
	for _, e := range p.active {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].order < entries[j].order })
	s := LSPWorkProgressSnapshot{LastCompleted: p.lastCompleted, Limited: p.limited, Closed: p.closed}
	for _, e := range entries {
		v := e.value
		v.Token = append(json.RawMessage(nil), v.Token...)
		if v.Percentage != nil {
			n := *v.Percentage
			v.Percentage = &n
		}
		s.Active = append(s.Active, v)
	}
	return s
}

func progressTokenKey(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 1024 {
		return "", false
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
		return "s:" + s, true
	}
	n, err := strconv.ParseInt(string(raw), 10, 32)
	if err != nil {
		return "", false
	}
	return "n:" + strconv.FormatInt(n, 10), true
}

func progressText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	end := limit
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return strings.Clone(s[:end])
}

func (c *LSPClient) routeWorkProgress(m *LSPMessage) bool {
	if m == nil || m.ID != nil || m.Method != "$/progress" {
		return false
	}
	p := &c.workProgress
	p.mu.Lock()
	enabled := p.updates != nil && !p.closed
	p.mu.Unlock()
	if !enabled {
		return false
	}
	// Discover only the discriminant first. Partial-result values may be arrays
	// or other objects; they are not work-done messages and must not be swallowed.
	var header struct {
		Token json.RawMessage `json:"token"`
		Value struct {
			Kind string `json:"kind"`
		} `json:"value"`
	}
	if json.Unmarshal(m.Params, &header) != nil {
		return false
	}
	kind := header.Value.Kind
	if kind != "begin" && kind != "report" && kind != "end" {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return true
	}
	changed := false
	defer func() {
		if changed {
			select {
			case p.updates <- struct{}{}:
			default:
			}
		}
	}()
	// End must retire a known operation even when the optional final message
	// is oversized. It is never retained, so the size guard must not leave a
	// completed job permanently stuck in the activity display.
	if kind == "end" {
		key, ok := progressTokenKey(header.Token)
		if !ok {
			return true
		}
		if e, exists := p.active[key]; exists {
			p.lastCompleted = e.value.Title
			delete(p.active, key)
			changed = true
		}
		return true
	}
	if len(m.Params) > 16<<10 {
		changed = !p.limited
		p.limited = true
		return true
	}
	var wire struct {
		Token json.RawMessage `json:"token"`
		Value struct {
			Title       string  `json:"title"`
			Message     *string `json:"message"`
			Percentage  *uint32 `json:"percentage"`
			Cancellable *bool   `json:"cancellable"`
		} `json:"value"`
	}
	if json.Unmarshal(m.Params, &wire) != nil {
		return true
	}
	key, ok := progressTokenKey(wire.Token)
	if !ok {
		changed = !p.limited
		p.limited = true
		return true
	}
	if wire.Value.Percentage != nil && *wire.Value.Percentage > 100 {
		return true
	}
	e, exists := p.active[key]
	switch kind {
	case "begin":
		if !exists && len(p.active) >= 64 {
			changed = !p.limited
			p.limited = true
			return true
		}
		if wire.Value.Title == "" {
			return true
		}
		p.sequence++
		e = lspProgressEntry{order: p.sequence, value: LSPWorkProgress{Token: append(json.RawMessage(nil), wire.Token...), Title: progressText(wire.Value.Title, 256)}}
	case "report":
		if !exists {
			return true
		}
	}
	if wire.Value.Message != nil {
		e.value.Message = progressText(*wire.Value.Message, 512)
	}
	if wire.Value.Percentage != nil {
		e.value.Percentage = wire.Value.Percentage
	}
	if wire.Value.Cancellable != nil {
		e.value.Cancellable = *wire.Value.Cancellable
	}
	if old, ok := p.active[key]; ok && kind == "report" && sameWorkProgress(old.value, e.value) {
		return true
	}
	p.active[key] = e
	changed = true
	return true
}

func sameWorkProgress(a, b LSPWorkProgress) bool {
	if a.Title != b.Title || a.Message != b.Message || a.Cancellable != b.Cancellable {
		return false
	}
	if a.Percentage == nil || b.Percentage == nil {
		return a.Percentage == nil && b.Percentage == nil
	}
	return *a.Percentage == *b.Percentage
}

func (c *LSPClient) closeWorkProgress() {
	p := &c.workProgress
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.active = nil
	if p.updates != nil {
		close(p.updates)
	}
}
