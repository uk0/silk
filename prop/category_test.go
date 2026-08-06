package prop

import "testing"

// TestCategoryOfPropIDEventNames: a Go event-handler id (On + capitalised verb,
// the names the designer records and codegen binds) belongs in the 事件
// category. The keyword heuristics alone scattered them: "OnTextChanged"
// matched the appearance keyword "text", "OnValueChanged" the behaviour
// keyword "value", and "OnToggle" matched nothing at all, so event rows landed
// under 外观/行为/常规 depending on the verb.
func TestCategoryOfPropIDEventNames(t *testing.T) {
	events := []string{
		"OnClick", "OnChanged", "OnToggled", "OnValueChanged", "OnTextChanged",
		"OnSearch", "OnRatingChanged", "OnDateChanged", "OnColorChanged",
		"OnSelect", "OnChange", "OnSelectionChanged", "OnClose", "OnNavigate",
		"OnSectionToggle", "OnItemClick", "OnTabChanged", "OnToggle",
	}
	for _, id := range events {
		if got := categoryOfPropID(id); got != "events" {
			t.Errorf("categoryOfPropID(%q) = %q, want \"events\"", id, got)
		}
	}
}

// TestCategoryOfPropIDNonEvents: the On-prefix rule must not swallow ordinary
// properties. "Online"/"OnlyRead" are not event names (no capital after "On"),
// and the existing keyword classification for other ids is unchanged.
func TestCategoryOfPropIDNonEvents(t *testing.T) {
	cases := map[string]string{
		"Online":     "general",
		"OnlyRead":   "general",
		"On":         "general",
		"width":      "layout",
		"text":       "appearance",
		"enabled":    "behavior",
		"tag":        "general",
		"font_color": "appearance",
	}
	for id, want := range cases {
		if got := categoryOfPropID(id); got != want {
			t.Errorf("categoryOfPropID(%q) = %q, want %q", id, got, want)
		}
	}
}
