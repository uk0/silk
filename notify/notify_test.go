package notify

import "testing"

// Headless limitation: Notify and Alert reach the OS notification centre
// through beeep, which needs a display/desktop session. Calling them on a
// headless runner hangs or fails, so they are NOT invoked here. Instead the
// compile-time assertions below pin their public signatures (no call, just an
// assignment), and only the pure AlarmMessage formatter is behaviour-tested.
var (
	_ func(string, string) error = Notify
	_ func(string, string) error = Alert
)

func TestAlarmMessage(t *testing.T) {
	tests := []struct {
		name      string
		tagName   string
		severity  string
		value     string
		wantTitle string
		wantBody  string
	}{
		{
			name:      "critical",
			tagName:   "Boiler.Temp",
			severity:  "CRITICAL",
			value:     "95.3",
			wantTitle: "[CRITICAL] Boiler.Temp",
			wantBody:  "Boiler.Temp = 95.3",
		},
		{
			name:      "low",
			tagName:   "Tank.Level",
			severity:  "LOW",
			value:     "12",
			wantTitle: "[LOW] Tank.Level",
			wantBody:  "Tank.Level = 12",
		},
		{
			name:      "empty fields",
			tagName:   "",
			severity:  "",
			value:     "",
			wantTitle: "[] ",
			wantBody:  " = ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			title, body := AlarmMessage(tt.tagName, tt.severity, tt.value)
			if title != tt.wantTitle {
				t.Errorf("title = %q, want %q", title, tt.wantTitle)
			}
			if body != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
		})
	}
}
