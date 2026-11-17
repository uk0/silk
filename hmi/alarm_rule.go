package hmi

import (
	"cmp"
	"encoding/json"
	"fmt"
	"time"

	"github.com/uk0/silk/point"
)

const MaxAlarmRules = 256

type AlarmCondition string

const (
	AlarmHigh  AlarmCondition = "high" // raise >= threshold, return <= reset
	AlarmLow   AlarmCondition = "low"  // raise <= threshold, return >= reset
	AlarmMatch AlarmCondition = "is"   // bool/string equality; the inverse returns
)

type AlarmPriority string

const (
	AlarmInfo     AlarmPriority = "info"
	AlarmWarning  AlarmPriority = "warning"
	AlarmCritical AlarmPriority = "critical"
)

// AlarmRule is an authored, read-only rule over one existing point. Numeric
// rules require disjoint trip/reset thresholds; a match rule has no reset
// parameter. Delays are elapsed observation time, never source timestamps.
type AlarmRule struct {
	ID        string         `json:"id"`
	Point     string         `json:"point"`
	Condition AlarmCondition `json:"condition"`
	Priority  AlarmPriority  `json:"priority"`
	Threshold any            `json:"threshold"`
	Reset     any            `json:"reset,omitempty"`
	RaiseMS   int            `json:"raise_ms,omitempty"`
	ClearMS   int            `json:"clear_ms,omitempty"`
	Latch     bool           `json:"latch,omitempty"`
}

type compiledAlarmRule struct {
	id, point    string
	kind         point.Kind
	condition    AlarmCondition
	priority     AlarmPriority
	trip, reset  any
	raise, clear time.Duration
	latch        bool
}

func compileAlarmRules(rules []AlarmRule, points map[string]point.Kind) ([]compiledAlarmRule, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	if len(rules) > MaxAlarmRules {
		return nil, fmt.Errorf("hmi: too many alarm rules")
	}
	ids := make(map[string]bool, len(rules))
	out := make([]compiledAlarmRule, 0, len(rules))
	for _, r := range rules {
		kind := points[r.Point]
		if !validID(r.ID) || ids[r.ID] || !kind.Valid() ||
			r.RaiseMS < 0 || r.RaiseMS > 86400000 || r.ClearMS < 0 || r.ClearMS > 86400000 {
			return nil, fmt.Errorf("hmi: invalid or duplicate alarm rule %q", r.ID)
		}
		switch r.Priority {
		case AlarmInfo, AlarmWarning, AlarmCritical:
		default:
			return nil, fmt.Errorf("hmi: invalid priority for alarm %q", r.ID)
		}
		trip, err := alarmScalar(kind, r.Threshold)
		if err != nil {
			return nil, fmt.Errorf("hmi: alarm %q threshold: %w", r.ID, err)
		}
		var reset any
		switch r.Condition {
		case AlarmMatch:
			if (kind != point.Bool && kind != point.String) || r.Reset != nil {
				return nil, fmt.Errorf("hmi: alarm %q match requires bool/string and no reset threshold", r.ID)
			}
		case AlarmHigh, AlarmLow:
			if kind != point.Int && kind != point.Float {
				return nil, fmt.Errorf("hmi: numeric alarm %q requires an int or float point", r.ID)
			}
			reset, err = alarmScalar(kind, r.Reset)
			if err != nil {
				return nil, fmt.Errorf("hmi: alarm %q reset: %w", r.ID, err)
			}
			order := compareAlarmNumbers(trip, reset)
			if (r.Condition == AlarmHigh && order <= 0) || (r.Condition == AlarmLow && order >= 0) {
				return nil, fmt.Errorf("hmi: alarm %q trip and reset overlap", r.ID)
			}
		default:
			return nil, fmt.Errorf("hmi: invalid condition for alarm %q", r.ID)
		}
		ids[r.ID] = true
		out = append(out, compiledAlarmRule{
			id: r.ID, point: r.Point, kind: kind, condition: r.Condition, priority: r.Priority,
			trip: trip, reset: reset, raise: time.Duration(r.RaiseMS) * time.Millisecond,
			clear: time.Duration(r.ClearMS) * time.Millisecond, latch: r.Latch,
		})
	}
	return out, nil
}

// Use the SDK's exact numeric conversions but do not implicitly interpret
// authored strings/bools as numbers, or numbers as bools.
func alarmScalar(kind point.Kind, raw any) (any, error) {
	switch kind {
	case point.Bool:
		if _, ok := raw.(bool); ok {
			return raw, nil
		}
	case point.String:
		if s, ok := raw.(string); ok && len(s) <= 4096 {
			return s, nil
		}
	case point.Int, point.Float:
		switch raw.(type) {
		case int, int64, float32, float64, json.Number:
			return point.Convert(raw, kind)
		}
	}
	return nil, fmt.Errorf("expected a %s scalar", kind)
}

func compareAlarmNumbers(a, b any) int {
	if i, ok := a.(int64); ok {
		return cmp.Compare(i, b.(int64))
	}
	return cmp.Compare(a.(float64), b.(float64))
}

func (r compiledAlarmRule) trips(raw any) bool {
	switch r.condition {
	case AlarmHigh:
		return compareAlarmNumbers(raw, r.trip) >= 0
	case AlarmLow:
		return compareAlarmNumbers(raw, r.trip) <= 0
	default:
		return raw == r.trip
	}
}

func (r compiledAlarmRule) resets(raw any) bool {
	switch r.condition {
	case AlarmHigh:
		return compareAlarmNumbers(raw, r.reset) <= 0
	case AlarmLow:
		return compareAlarmNumbers(raw, r.reset) >= 0
	default:
		return raw != r.trip
	}
}
