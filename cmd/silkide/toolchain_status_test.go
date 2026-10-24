package main

import (
	"context"
	"github.com/uk0/silk/gui"
	"io"
	"strings"
	"testing"
)

func TestToolStatusOutputBounded(t *testing.T) {
	var b probeBuffer
	n, err := b.Write([]byte(strings.Repeat("x", 20000)))
	if n != 20000 || err != nil || b.Len() != 4096 {
		t.Fatal(n, err, b.Len())
	}
	b.Write([]byte("more"))
	if b.Len() != 4096 {
		t.Fatal(b.Len())
	}
}
func TestToolStatusAndRestartMenuDiscoverable(t *testing.T) {
	w := testWorkbench(t)
	m := buildWorkbenchSubMenu("Tools", w.tabs, w.canvas)
	defer releaseWorkbenchMenu(m)
	for _, id := range []string{"tools.status", "tools.restartLSP"} {
		b := menuCommandButton(m, id)
		if b == nil || !b.IsEnabled() || b.Icon() == nil {
			t.Fatal("missing tool command", id)
		}
	}
}

func TestToolStatusLongReportIsScrollableAndReadOnly(t *testing.T) {
	text := strings.Repeat("C:/tools/long-path/gopls.exe: unavailable\n", 300)
	d := newToolchainStatusDialog(nil, text)
	defer d.SetParent(nil)
	report, ok := d.Content().(*gui.Edit)
	if !ok || !report.IsReadOnly() || report.Text() != text {
		t.Fatal("status must retain a selectable read-only report")
	}
	d.Layout()
	if report.Width() <= 0 || report.Height() <= 0 || report.Height() >= d.Height() {
		t.Fatal("report must leave room for dialog buttons")
	}
	report.OnTextInput("overwrite")
	if report.Text() != text {
		t.Fatal("status report mutated")
	}
}

func TestToolStatusCopyCannotBypassOutputLimit(t *testing.T) {
	var b probeBuffer
	n, err := io.Copy(&b, io.LimitReader(strings.NewReader(strings.Repeat("x", 20000)), 20000))
	if err != nil || n != 20000 || b.Len() != 4096 {
		t.Fatalf("io.Copy bypassed cap: n=%d retained=%d err=%v", n, b.Len(), err)
	}
}

func TestToolStatusRecoveryDoesNotClaimMissingConnectedServer(t *testing.T) {
	// An already cancelled probe context avoids launching external tools.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report := collectToolchainStatus(ctx, true)
	for _, want := range []string{"Language service connected: true", "Language server recovery (if unavailable):", "make ide and launch bin/silkide"} {
		if !strings.Contains(report, want) {
			t.Fatalf("missing status/recovery text %q", want)
		}
	}
	if strings.Contains(report, "Missing gopls:") {
		t.Fatal("generic recovery footer must not claim a connected server is missing")
	}
}
