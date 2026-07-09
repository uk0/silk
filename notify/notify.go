// Package notify sends native OS desktop notifications, giving the desktop
// build parity with other native apps: events such as alarms surface through
// the host notification centre instead of only inside the app window.
//
// It is a thin adapter over github.com/gen2brain/beeep. Notify and Alert reach
// the OS and therefore need a display/desktop session to succeed; AlarmMessage
// is a pure formatter with no side effects.
package notify

import (
	"fmt"

	"github.com/gen2brain/beeep"
)

// Notify shows a standard desktop notification. The icon path is left empty so
// the host default is used.
func Notify(title, message string) error {
	return beeep.Notify(title, message, "")
}

// Alert shows a desktop notification that also plays the system alert sound,
// for events that need the user's attention.
func Alert(title, message string) error {
	return beeep.Alert(title, message, "")
}

// AlarmMessage formats an alarm into a notification title and body. It is a
// pure helper: it takes plain strings, imports no alarm/core types, and has no
// side effects, so it is safe to unit-test in a headless environment.
func AlarmMessage(tagName, severity, value string) (title, body string) {
	title = fmt.Sprintf("[%s] %s", severity, tagName)
	body = fmt.Sprintf("%s = %s", tagName, value)
	return title, body
}
