package gui

import (
	"cmp"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
)

type processPointRow struct {
	state   hmi.PointState
	cells   [8]string
	natural int
}
type processPointModel struct {
	rows      []*processPointRow
	byName    map[string]*processPointRow
	column    int
	ascending bool
}

func newProcessPointModel(states []hmi.PointState) *processPointModel {
	m := &processPointModel{byName: make(map[string]*processPointRow, len(states)), column: -1}
	for i, s := range states {
		r := &processPointRow{state: s, natural: i}
		r.refresh()
		m.rows = append(m.rows, r)
		m.byName[s.Definition.Name] = r
	}
	return m
}
func processPointText(text string) string {
	var out strings.Builder
	out.Grow(min(len(text), 512))
	count := 0
	for _, r := range text {
		if count == 160 {
			out.WriteRune('…')
			return out.String()
		}
		if r == '\r' || r == '\n' {
			r = '↵'
		}
		out.WriteRune(r)
		count++
	}
	return out.String()
}

func processPointDisplay(raw any) string {
	if raw == nil {
		return "—"
	}
	if text, ok := raw.(string); ok {
		return processPointText(text)
	}
	// Engineering display only: keep arithmetic noise out of a narrow cell.
	// PointState, sorting, history records and the write editor retain full data.
	if value, ok := raw.(float64); ok {
		return strconv.FormatFloat(value, 'g', 12, 64)
	}
	return processPointText(fmt.Sprint(raw))
}
func processPointQuality(q core.Quality) string {
	switch q {
	case core.QualityGood:
		return i18n.T("Good")
	case core.QualityUncertain:
		return i18n.T("Uncertain")
	case core.QualityBad:
		return i18n.T("Bad")
	}
	return "Unknown"
}
func processPointWriter(s hmi.PointState) string {
	if s.Writer != "" {
		return "HTTP: " + s.Writer
	}
	return i18n.T("Manual / driver")
}
func (r *processPointRow) refresh() {
	s := r.state
	value := processPointDisplay(s.Value.Raw)
	at := "—"
	if !s.Value.Time.IsZero() {
		at = s.Value.Time.UTC().Format("2006-01-02 15:04:05.000Z")
	}
	r.cells = [8]string{s.Definition.Name, s.Definition.Category, string(s.Definition.Kind), value, s.Definition.Unit, processPointQuality(s.Value.Quality), at, processPointWriter(s)}
}
func (m *processPointModel) update(s hmi.PointState) bool {
	r, ok := m.byName[s.Definition.Name]
	if !ok || s.Revision < r.state.Revision {
		return false
	}
	if s.Token == r.state.Token && s.Writable == r.state.Writable && s.Writer == r.state.Writer {
		return false
	}
	r.state = s
	r.refresh()
	return true
}
func (m *processPointModel) RowCount() int    { return len(m.rows) }
func (m *processPointModel) ColumnCount() int { return 8 }
func (m *processPointModel) CellText(row, col int) string {
	if row < 0 || row >= len(m.rows) || col < 0 || col >= 8 {
		return ""
	}
	return m.rows[row].cells[col]
}
func (m *processPointModel) HeaderText(col int) string {
	if col < 0 || col >= 8 {
		return ""
	}
	return i18n.T([8]string{"Point", "Category", "Type", "Value", "Unit", "Quality", "Observed", "Writer"}[col])
}
func (m *processPointModel) ColumnWidth(col int) float64 {
	if col < 0 || col >= 8 {
		return 100
	}
	return [8]float64{128, 72, 72, 160, 68, 96, 190, 150}[col]
}
func (m *processPointModel) SortByColumn(col int, ascending bool) {
	m.column = col
	m.ascending = ascending
	m.resort()
}
func (m *processPointModel) RestoreOrder() {
	m.column = -1
	sort.SliceStable(m.rows, func(i, j int) bool { return m.rows[i].natural < m.rows[j].natural })
}
func (m *processPointModel) resort() {
	col := m.column
	if col < 0 || col >= 8 {
		return
	}
	sort.SliceStable(m.rows, func(i, j int) bool {
		a, b := m.rows[i], m.rows[j]
		c := 0
		if col == 6 {
			c = a.state.Value.Time.UTC().Compare(b.state.Value.Time.UTC())
		} else if col == 5 {
			c = cmp.Compare(a.state.Value.Quality, b.state.Value.Quality)
		} else if col == 3 && a.state.Definition.Kind == b.state.Definition.Kind {
			switch x := a.state.Value.Raw.(type) {
			case int64:
				if y, ok := b.state.Value.Raw.(int64); ok {
					c = cmp.Compare(x, y)
				} else {
					c = compareTableCells(a.cells[col], b.cells[col])
				}
			case float64:
				if y, ok := b.state.Value.Raw.(float64); ok {
					c = cmp.Compare(x, y)
				} else {
					c = compareTableCells(a.cells[col], b.cells[col])
				}
			default:
				c = compareTableCells(a.cells[col], b.cells[col])
			}
		} else {
			c = compareTableCells(a.cells[col], b.cells[col])
		}
		if c == 0 {
			return a.natural < b.natural
		}
		if !m.ascending {
			c = -c
		}
		return c < 0
	})
}

// An inline field is bounded independently of the persisted string-point size.
// Reject, rather than truncate, a value too large to edit safely in this dialog.
func processPointInlineText(s hmi.PointState) (string, error) {
	text := ""
	if s.Value.Raw != nil {
		if value, ok := s.Value.Raw.(string); ok {
			text = value
		} else {
			text = fmt.Sprint(s.Value.Raw)
		}
	}
	if len(text) > 8192 || !utf8.ValidString(text) {
		return "", fmt.Errorf("point value is too large or invalid UTF-8 for the inline editor; use the SDK")
	}
	return text, nil
}
