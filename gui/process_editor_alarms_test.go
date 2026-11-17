package gui

import (
	"encoding/json"
	"testing"
)

func TestProcessEditorAlarmRulesAreAtomicAndUndoable(t *testing.T) {
	e := NewProcessEditor()
	defer e.Close()
	if err := e.ApplyPointRows([][]string{
		{"DI11", "bool", "DI", "", "false", "0", ""},
		{"AI101", "float", "AI", "°C", "72.5", "0", ""},
		{"BI11", "int", "BI", "", "9007199254740992", "0", ""},
	}); err != nil {
		t.Fatal(err)
	}
	before := e.ProjectJSON()
	rows := [][]string{
		{"temperature", "BI11", "high", "critical", "9007199254740993", "9007199254740991", "100", "250", "true"},
		{"valve", "DI11", "is", "warning", "false", "", "0", "0", "false"},
	}
	if err := e.ApplyAlarmRows(rows); err != nil {
		t.Fatal(err)
	}
	applied := e.ProjectJSON()
	alarms := e.Diagram.Project().Alarms
	if len(alarms) != 2 || !alarms[0].Latch || alarms[0].Threshold != json.Number("9007199254740993") || alarms[1].Threshold != false {
		t.Fatal("typed authoring lost exact rule data", alarms)
	}
	rows[1][5] = "true"
	if err := e.ApplyAlarmRows(rows); err == nil || e.ProjectJSON() != applied {
		t.Fatal("malformed candidate partially applied")
	}
	e.Diagram.Undo()
	if e.ProjectJSON() != before {
		t.Fatal("full rule transaction did not undo exactly once")
	}
	e.Diagram.Redo()
	if e.ProjectJSON() != applied {
		t.Fatal("redo lost typed rules")
	}
	e.SetRunning(true)
	if err := e.ApplyAlarmRows(nil); err == nil || e.ProjectJSON() != applied {
		t.Fatal("runtime admitted authoring mutation")
	}
}
