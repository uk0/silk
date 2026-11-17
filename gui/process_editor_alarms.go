package gui

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/point"
)

const alarmRuleHelp = "high: threshold > reset; low: threshold < reset; is: bool/text with blank reset. Delays are milliseconds. Latch holds the returned alarm until acknowledged."

func init() {
	i18n.AddMany("zh", map[string]string{
		"Alarm rules": "报警规则", "Alarm ID": "报警 ID", "Condition": "触发条件",
		"Alarm priority": "优先级", "Threshold": "触发阈值", "Reset threshold": "复归阈值",
		"Raise ms": "触发延时 (ms)", "Clear ms": "复归延时 (ms)", "Latch": "锁存",
		alarmRuleHelp:                            "high：触发阈值 > 复归阈值；low：触发阈值 < 复归阈值；is：布尔或文本匹配，复归阈值留空。延时单位为毫秒。锁存报警复归后保留至确认。",
		"Alarm rules changed; reopen the editor": "报警规则所在的组态已变化，请重新打开编辑器",
	})
}

func alarmCell(kind point.Kind, row []string, col int) (any, error) {
	switch kind {
	case point.String:
		return rawCell(row, col), nil
	case point.Bool:
		switch cell(row, col) {
		case "true":
			return true, nil
		case "false":
			return false, nil
		}
	case point.Int, point.Float:
		text := cell(row, col)
		if json.Valid([]byte(text)) {
			return point.Convert(json.Number(text), kind)
		}
	}
	return nil, fmt.Errorf("expected a %s alarm threshold", kind)
}

func alarmDelay(row []string, col int) (int, error) {
	text := cell(row, col)
	if text == "" {
		return 0, nil
	}
	return strconv.Atoi(text)
}

// ApplyAlarmRows validates all rows and commits one undo transaction. It
// never creates a runtime or writes a point. Column order matches EditAlarmRules.
func (v *ProcessEditor) ApplyAlarmRows(rows [][]string) error {
	if v.closed || v.locked || v.running {
		return fmt.Errorf("hmi: authoring is locked")
	}
	if len(rows) > hmi.MaxAlarmRules {
		return fmt.Errorf("hmi: too many alarm rules")
	}
	p := v.Diagram.Project()
	p.Alarms = nil
	kinds := make(map[string]point.Kind, len(p.Points))
	for _, def := range p.Points {
		kinds[def.Name] = def.Kind
	}
	for i, row := range rows {
		r := hmi.AlarmRule{ID: cell(row, 0), Point: cell(row, 1), Condition: hmi.AlarmCondition(cell(row, 2)), Priority: hmi.AlarmPriority(cell(row, 3))}
		bad := func(err error) error { return fmt.Errorf("alarm row %d: %w", i+1, err) }
		kind := kinds[r.Point]
		if !kind.Valid() {
			return bad(fmt.Errorf("unknown point %q", r.Point))
		}
		var err error
		if r.Threshold, err = alarmCell(kind, row, 4); err != nil {
			return bad(err)
		}
		if r.Condition != hmi.AlarmMatch {
			if r.Reset, err = alarmCell(kind, row, 5); err != nil {
				return bad(err)
			}
		} else if rawCell(row, 5) != "" {
			return bad(fmt.Errorf("match rule must leave reset blank"))
		}
		if r.RaiseMS, err = alarmDelay(row, 6); err != nil {
			return bad(err)
		}
		if r.ClearMS, err = alarmDelay(row, 7); err != nil {
			return bad(err)
		}
		if s := cell(row, 8); s != "" {
			if r.Latch, err = strconv.ParseBool(s); err != nil {
				return bad(err)
			}
		}
		p.Alarms = append(p.Alarms, r)
	}
	return v.Diagram.ApplyProject(p)
}

func (v *ProcessEditor) EditAlarmRules() {
	p := v.Diagram.Project()
	original := v.ProjectJSON()
	rows := make([][]string, 0, len(p.Alarms))
	for _, a := range p.Alarms {
		reset := ""
		if a.Reset != nil {
			reset = fmt.Sprint(a.Reset)
		}
		rows = append(rows, []string{
			a.ID, a.Point, string(a.Condition), string(a.Priority),
			fmt.Sprint(a.Threshold), reset, strconv.Itoa(a.RaiseMS), strconv.Itoa(a.ClearMS), strconv.FormatBool(a.Latch),
		})
	}
	var newRow []string
	if len(p.Points) > 0 && len(p.Alarms) < hmi.MaxAlarmRules {
		d := p.Points[0]
		newRow = []string{"ALARM_NEW", d.Name, "high", "warning", "1", "0", "0", "0", "false"}
		switch d.Kind {
		case point.Bool:
			newRow[2], newRow[4], newRow[5] = "is", "true", ""
		case point.String:
			newRow[2], newRow[4], newRow[5] = "is", "", ""
		}
	}
	headers := []string{"Alarm ID", "Point", "Condition", "Alarm priority", "Threshold", "Reset threshold", "Raise ms", "Clear ms", "Latch"}
	for i, header := range headers {
		headers[i] = i18n.T(header)
	}
	v.editRows(i18n.T("Alarm rules"), headers, rows, newRow, func(rows [][]string) error {
		if v.ProjectJSON() != original {
			return fmt.Errorf("%s", i18n.T("Alarm rules changed; reopen the editor"))
		}
		return v.ApplyAlarmRows(rows)
	}, i18n.T(alarmRuleHelp))
}
