package decl

import (
	"encoding/hex"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/uk0/silk/core"
)

// ApplyPersistedProperties restores a designer props block through the same
// data-only codec and dependency ordering used by Node.Build. Only writable
// properties explicitly enumerated by obj are accepted; unknown or malformed
// values are ignored. Item lists precede selection, ranges precede values, and
// date components are committed together. The input map is not modified.
func ApplyPersistedProperties(obj core.IEnumProperties, values map[string]string) {
	if obj == nil || len(values) == 0 {
		return
	}
	list := &persistedPropertyList{values: values}
	obj.EnumProperties(list)
	handled := applyCompoundProperties(obj, list.props)
	for _, p := range list.props {
		if !handled[p.Name] {
			applyPersistedProperty(obj, p.Name, p.Value.(Persisted).Source)
		}
	}
}

type persistedPropertyList struct {
	values map[string]string
	props  []Prop
}

func (p *persistedPropertyList) AddProperty(id string, _, set interface{}) {
	raw, ok := p.values[id]
	if !ok || set == nil {
		return
	}
	setter := reflect.ValueOf(set)
	typ := setter.Type()
	if typ.Kind() != reflect.Func || setter.IsNil() || typ.NumIn() != 1 || typ.NumOut() != 0 {
		return
	}
	p.props = append(p.props, Prop{Name: id, Value: Persisted{Source: raw}})
}

// Only properties explicitly exposed by the widget's property sheet may be
// restored. No setter names, expressions or event bodies are evaluated.
func applyPersistedProperty(obj interface{}, id, raw string) {
	if w, ok := obj.(core.IEnumProperties); ok {
		w.EnumProperties(&persistedProperty{id: id, raw: raw})
	}
}

type persistedProperty struct {
	id, raw  string
	foldCase bool
}

func (p *persistedProperty) AddProperty(id string, _, set interface{}) {
	if (id != p.id && (!p.foldCase || !strings.EqualFold(id, p.id))) || set == nil {
		return
	}
	setter := reflect.ValueOf(set)
	typ := setter.Type()
	if typ.Kind() != reflect.Func || setter.IsNil() || typ.NumIn() != 1 || typ.NumOut() != 0 {
		return
	}
	raw := p.raw
	if p.foldCase && typ.In(0).Kind() != reflect.String {
		// TDoc's untyped literal dialect returns scalar tokens as strings.
		if text, err := strconv.Unquote(raw); err == nil {
			raw = text
		}
	}
	if value, ok := decodePersisted(raw, typ.In(0)); ok {
		setter.Call([]reflect.Value{value})
	}
}

func decodePersisted(raw string, typ reflect.Type) (reflect.Value, bool) {
	if typ == reflect.TypeOf([]string{}) {
		items, ok := decodeStringItems(raw)
		if !ok {
			return reflect.Value{}, false
		}
		return reflect.ValueOf(items), true
	}
	if isPaintRGBAType(typ) {
		if (len(raw) != 7 && len(raw) != 9) || raw[0] != '#' {
			return reflect.Value{}, false
		}
		rgba, err := hex.DecodeString(raw[1:])
		if err != nil {
			return reflect.Value{}, false
		}
		color := reflect.New(typ).Elem()
		channels := []uint8{rgba[0], rgba[1], rgba[2], 255}
		if len(rgba) == 4 {
			channels[3] = rgba[3]
		}
		for i, name := range []string{"R", "G", "B", "A"} {
			color.FieldByName(name).SetUint(uint64(channels[i]))
		}
		return color, true
	}
	value := reflect.New(typ).Elem()
	switch typ.Kind() {
	case reflect.String:
		s, err := strconv.Unquote(raw)
		if err != nil {
			return reflect.Value{}, false
		}
		value.SetString(s)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return reflect.Value{}, false
		}
		value.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(raw, 10, typ.Bits())
		if err != nil {
			return reflect.Value{}, false
		}
		value.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, typ.Bits())
		if err != nil {
			return reflect.Value{}, false
		}
		value.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, err := strconv.ParseFloat(raw, typ.Bits())
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return reflect.Value{}, false
		}
		value.SetFloat(n)
	default:
		return reflect.Value{}, false
	}
	return value, true
}

// Literal DSL properties use the same explicit property-sheet allowlist as
// designer persisted values. Never reflect arbitrary Set<Name> methods or
// evaluate expressions/event bodies from a document.
func applyLiteralProperty(obj interface{}, id string, value interface{}) {
	if w, ok := obj.(core.IEnumProperties); ok {
		raw, ok := literalPropertySource(value)
		if ok {
			w.EnumProperties(&persistedProperty{id: id, raw: raw, foldCase: true})
		}
	}
}

func literalPropertySource(value interface{}) (string, bool) {
	if items, ok := value.([]string); ok {
		raw, err := core.PersistString(items)
		return raw, err == nil
	}
	if value == nil {
		return "", false
	}
	v := reflect.ValueOf(value)
	if isPaintRGBAType(v.Type()) {
		rgba := make([]byte, 4)
		for i, name := range []string{"R", "G", "B", "A"} {
			rgba[i] = byte(v.FieldByName(name).Uint())
		}
		if rgba[3] == 255 {
			rgba = rgba[:3]
		}
		return "#" + strings.ToUpper(hex.EncodeToString(rgba)), true
	}
	switch v.Kind() {
	case reflect.String:
		return strconv.Quote(v.String()), true
	case reflect.Bool:
		return strconv.FormatBool(v.Bool()), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(v.Int(), 10), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return strconv.FormatUint(v.Uint(), 10), true
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return "", false
		}
		return strconv.FormatFloat(f, 'g', -1, v.Type().Bits()), true
	}
	return "", false
}

// Interdependent setters must see the final tuple before individual setters
// run: Value=250 before Max=300 otherwise clamps permanently to the factory's
// default 100; Day=31 before Month=January can lose a day in September.
func applyCompoundProperties(obj interface{}, props []Prop) map[string]bool {
	handled := make(map[string]bool)
	if setter, ok := obj.(interface{ SetItems([]string) }); ok {
		for _, p := range props {
			if p.Name != "items" {
				continue
			}
			var items []string
			var valid bool
			switch value := p.Value.(type) {
			case Lit:
				items, valid = value.V.([]string)
				if !valid {
					if raw, ok := value.V.(string); ok {
						items, valid = decodeStringItems(raw)
					}
				}
			case Persisted:
				items, valid = decodeStringItems(value.Source)
			}
			if valid {
				setter.SetItems(items)
				handled[p.Name] = true
			}
		}
	}
	if ranged, ok := obj.(interface {
		Min() float64
		Max() float64
		SetRange(float64, float64)
	}); ok {
		min, max, changed := ranged.Min(), ranged.Max(), false
		for _, p := range props {
			value, valid := propertyNumber(p.Value)
			if !valid {
				continue
			}
			switch strings.ToLower(p.Name) {
			case "min", "最小值":
				min, changed = value, true
				handled[p.Name] = true
			case "max", "最大值":
				max, changed = value, true
				handled[p.Name] = true
			}
		}
		if changed {
			ranged.SetRange(min, max)
		}
	}
	// Integer controls cannot use the float range capability: going through
	// float64 would also corrupt int64 bounds beyond 2^53. Commit their final
	// range before Value, for designer reload, preview and declarative builds.
	if ranged, ok := obj.(interface {
		Min() int
		Max() int
		SetRange(int, int)
	}); ok {
		min, max, changed := ranged.Min(), ranged.Max(), false
		for _, p := range props {
			switch strings.ToLower(p.Name) {
			case "min", "最小值", "max", "最大值":
				value, valid := propertyInteger(p.Value)
				if !valid {
					continue
				}
				if strings.ToLower(p.Name) == "min" || p.Name == "最小值" {
					min = value
				} else {
					max = value
				}
				changed, handled[p.Name] = true, true
			}
		}
		if changed {
			ranged.SetRange(min, max)
		}
	}
	if date, ok := obj.(interface {
		Year() int
		Month() int
		Day() int
		SetDate(int, int, int)
	}); ok {
		year, month, day, changed := date.Year(), date.Month(), date.Day(), false
		for _, p := range props {
			value, valid := propertyNumber(p.Value)
			// Do not truncate fractional dates or overflow int on 32-bit consumers.
			limit := math.Ldexp(1, strconv.IntSize-1)
			if !valid || math.Trunc(value) != value || value < -limit || value >= limit {
				continue
			}
			switch strings.ToLower(p.Name) {
			case "year":
				year, changed = int(value), true
				handled[p.Name] = true
			case "month":
				month, changed = int(value), true
				handled[p.Name] = true
			case "day":
				day, changed = int(value), true
				handled[p.Name] = true
			}
		}
		if changed {
			date.SetDate(year, month, day)
		}
	}
	return handled
}

// propertyInteger preserves typed and persisted integers without a float64
// round trip. Reject fractions, overflow and non-data expressions rather than
// silently truncating, rounding or wrapping them (including on 32-bit hosts).
func propertyInteger(value Value) (int, bool) {
	parse := func(raw string) (int, bool) {
		n, err := strconv.ParseInt(raw, 10, strconv.IntSize)
		return int(n), err == nil
	}
	switch v := value.(type) {
	case Persisted:
		return parse(v.Source)
	case Lit:
		if v.V == nil {
			return 0, false
		}
		rv := reflect.ValueOf(v.V)
		switch rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			n := rv.Int()
			return int(n), n >= int64(math.MinInt) && n <= int64(math.MaxInt)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			n := rv.Uint()
			return int(n), n <= uint64(math.MaxInt)
		case reflect.Float32, reflect.Float64:
			n := rv.Float()
			limit := math.Ldexp(1, strconv.IntSize-1)
			if math.Trunc(n) == n && n >= -limit && n < limit {
				return int(n), true
			}
		case reflect.String:
			return parse(rv.String())
		}
	}
	return 0, false
}

func propertyNumber(value Value) (float64, bool) {
	switch v := value.(type) {
	case Lit:
		return asFloat(v.V)
	case Persisted:
		return asFloat(v.Source)
	}
	return 0, false
}

// The persisted string-list syntax is data only: a bracketed sequence of
// quoted strings. No identifiers, expressions, trailing bytes or executable
// deserialization hooks are accepted.
func decodeStringItems(raw string) ([]string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '[' || raw[len(raw)-1] != ']' {
		return nil, false
	}
	rest := strings.TrimSpace(raw[1 : len(raw)-1])
	items := make([]string, 0)
	for rest != "" {
		if rest[0] != '"' {
			return nil, false
		}
		quoted, err := strconv.QuotedPrefix(rest)
		if err != nil {
			return nil, false
		}
		text, err := strconv.Unquote(quoted)
		if err != nil {
			return nil, false
		}
		items = append(items, text)
		tail := rest[len(quoted):]
		rest = strings.TrimSpace(tail)
		if rest != "" && len(tail) == len(rest) {
			return nil, false
		}
	}
	return items, true
}

// Keep decl and silkgen independent of the native renderer. The property
// sheet supplies the real runtime type; accept only paint's exported 8-bit
// RGBA value, not arbitrary structs, aliases from other packages or hooks.
func isPaintRGBAType(typ reflect.Type) bool {
	if typ.Kind() != reflect.Struct || typ.PkgPath() != "github.com/uk0/silk/paint" || typ.Name() != "Color" || typ.NumField() != 4 {
		return false
	}
	for _, name := range []string{"R", "G", "B", "A"} {
		field, ok := typ.FieldByName(name)
		if !ok || field.PkgPath != "" || field.Type != reflect.TypeOf(uint8(0)) {
			return false
		}
	}
	return true
}
