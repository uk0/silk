package alarmbridge

// Bridge from a core.AlarmDB's transition stream to two side effects: an event
// sink (audit log) and a user-facing notifier. It depends ONLY on core plus the
// stdlib; the concrete eventlog / notify implementations are injected as plain
// funcs so this package stays free of any UI or logging dependency.
//
// core.AlarmDB.Subscribe fans out every transition — raise, re-raise
// (escalation), ack, and clear. A notifier must react to the ones a human cares
// about and stay quiet on the rest, so Watch filters the stream:
//
//   - fire on an active, raised alarm (Active && Severity.IsAlarm())
//   - skip cleared / inactive states (a return-to-normal is not an alarm)
//   - skip pure acknowledgements (Acked): an operator ack must not re-notify.
//     An escalation re-raise resets Acked in the db, so a genuinely worse
//     condition still fires.

import (
	"fmt"

	"github.com/uk0/silk/core"
)

// formatAlarm renders a raised alarm into a notification title and message. It
// is pure — no I/O, no clock — so it can be unit-tested in isolation.
//
//	title   = "[<severity>] <tag>"   e.g. "[HiHi] TIC-101"
//	message = "<tag> <severity> = <value>" e.g. "TIC-101 HiHi = 160"
func formatAlarm(s core.AlarmState) (title, message string) {
	title = fmt.Sprintf("[%s] %s", s.Severity, s.Tag)
	message = fmt.Sprintf("%s %s = %g", s.Tag, s.Severity, s.Value)
	return title, message
}

// Watch subscribes to db and, for every state change that represents an active
// raised alarm, records an audit event via onEvent(tag, message) and raises a
// user notification via notify(title, message). Both sinks are optional; a nil
// sink is skipped. Cleared/inactive states and bare acknowledgements are
// filtered out (see the package doc). The returned func unsubscribes and is
// safe to call more than once (it wraps core.AlarmDB's idempotent CancelFunc).
func Watch(db *core.AlarmDB, onEvent func(source, message string), notify func(title, body string)) func() {
	return db.Subscribe(func(s core.AlarmState) {
		if !s.Active || !s.Severity.IsAlarm() || s.Acked {
			return
		}
		title, message := formatAlarm(s)
		if onEvent != nil {
			onEvent(s.Tag, message)
		}
		if notify != nil {
			notify(title, message)
		}
	})
}
