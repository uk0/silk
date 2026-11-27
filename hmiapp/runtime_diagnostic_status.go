package hmiapp

import (
	"fmt"
	"strings"

	"github.com/uk0/silk/async"
	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/hmi"
	"github.com/uk0/silk/i18n"
)

func init() {
	i18n.AddMany("zh", map[string]string{
		"1 source error":            "1 个数据源错误",
		"%d source errors":          "%d 个数据源错误",
		"storage maintenance error": "存储维护错误",
		"Running · shared points / bounded HTTP / quality / staleness": "运行中 · 共享点位 / HTTP 采集 / 质量与过期状态",
	})
}

// watchDiagnostics is bound to one session on the UI thread. The runtime
// callback only invalidates the latest-value queue; UI work and diagnostic
// reads never run on the HTTP or storage workers.
func (s *session) watchDiagnostics(post func(func()), render func(hmi.RuntimeDiagnostics)) {
	s.unwatchDiagnostics()
	rt := s.runtime
	latest, _ := async.NewLatest(post, func(struct{}) { render(rt.Diagnostics()) })
	cancel := rt.SubscribeDiagnosticChanges(func() { latest.Publish(struct{}{}) })
	s.stopDiagnostics = func() {
		cancel()
		latest.Close()
	}
	render(rt.Diagnostics())
}

func (s *session) unwatchDiagnostics() {
	if s.stopDiagnostics != nil {
		s.stopDiagnostics()
		s.stopDiagnostics = nil
	}
}

// Put faults at the start of the single-line label: clipping a long source
// filename or normal running caption must not conceal a fault in a narrow pane.
func runtimeDiagnosticStatusText(base string, d hmi.RuntimeDiagnostics, showHealthyCount bool) string {
	var parts [3]string
	n := 0
	switch count := len(d.Sources); count {
	case 0:
	case 1:
		parts[n] = i18n.T("1 source error")
		n++
	default:
		parts[n] = fmt.Sprintf(i18n.T("%d source errors"), count)
		n++
	}
	if d.RetentionError != "" {
		parts[n] = i18n.T("storage maintenance error")
		n++
	}
	if n == 0 && !showHealthyCount {
		return base
	}
	parts[n] = base
	n++
	if len(d.Sources) == 0 && showHealthyCount {
		parts[n] = fmt.Sprintf(i18n.T("%d source errors"), 0)
		n++
	}
	return strings.Join(parts[:n], " · ")
}

func setRuntimeDiagnosticStatus(label *gui.Label, base string, d hmi.RuntimeDiagnostics) {
	display := runtimeDiagnosticStatusText(base, d, false)
	if label.Text() != display {
		label.SetText(display)
	}
}
