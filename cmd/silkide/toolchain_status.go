package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/uk0/silk/gui"
	"github.com/uk0/silk/i18n"
	"github.com/uk0/silk/internal/idetools"
	"github.com/uk0/silk/internal/processutil"
)

var toolStatusPending bool // UI thread only; repeated clicks do not spawn probes.

func showToolchainStatus(parent gui.IWidget) {
	if toolStatusPending {
		return
	}
	toolStatusPending = true
	connected := globalLSP != nil
	activity := lspActivityReport()
	_, err := gui.RunAsync(context.Background(), nil, func(ctx context.Context) (string, error) {
		return activity + collectToolchainStatus(ctx, connected), nil
	}, func(text string, err error) {
		toolStatusPending = false
		if err != nil {
			text = err.Error()
		}
		newToolchainStatusDialog(parent, text).ShowModal()
	})
	if err != nil {
		toolStatusPending = false
		silkideToast(err.Error(), gui.ToastWarning)
	}
}

// A scrollable, selectable report keeps long paths/errors inside the dialog.
// A plain wrapped Label can grow beyond the screen and obscure its OK button.
func newToolchainStatusDialog(parent gui.IWidget, text string) *gui.Dialog {
	d := gui.NewDialog(i18n.T("Toolchain Status"), parent)
	report := gui.NewEdit()
	report.SetMultiLine(true)
	report.SetWrap(true)
	report.SetReadOnly(true)
	report.SetFocusPolicy(gui.TabFocus)
	report.SetText(text)
	report.SetCaretPos(0)
	d.SetContent(report)
	d.AddButton(i18n.T("Close"), gui.DialogOK)
	d.SetSize(780, 520)
	d.SetInitialFocus(report)
	return d
}

// Do not embed bytes.Buffer: its promoted ReadFrom lets exec's io.Copy bypass
// Write and retain unbounded child output.
type probeBuffer struct{ data bytes.Buffer }

func (b *probeBuffer) Len() int       { return b.data.Len() }
func (b *probeBuffer) String() string { return b.data.String() }

func (b *probeBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 4096 - b.Len()
	if remaining > 0 {
		_, _ = b.data.Write(p[:min(n, remaining)])
	}
	return n, nil
}
func probeTool(ctx context.Context, path string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.WaitDelay = 500 * time.Millisecond
	processutil.NoConsoleWindow(cmd)
	var out probeBuffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	text := strings.TrimSpace(out.String())
	if err != nil {
		return fmt.Sprintf("%s\n%v", text, err)
	}
	return text
}
func collectToolchainStatus(ctx context.Context, connected bool) string {
	var s strings.Builder
	fmt.Fprintf(&s, "Language service connected: %t\nBundled gopls: %s (compiler %s)\n\n", connected, idetools.GoplsVersion, idetools.BuildToolchain)
	for _, spec := range []struct {
		name, role string
		args       []string
	}{{"go", "required Go SDK (external)", []string{"version"}}, {"gopls", "included in complete IDE bundles", []string{"version"}}, {"dlv", "optional debugger (external)", []string{"version"}}, {"git", "optional version control (external)", []string{"--version"}}} {
		path, source := "", "PATH"
		var err error
		if spec.name == "gopls" {
			var tool idetools.Tool
			tool, err = idetools.Resolve()
			path, source = tool.Path, tool.Source
		} else {
			path, err = exec.LookPath(spec.name)
		}
		fmt.Fprintf(&s, "%s - %s\n", spec.name, spec.role)
		if err != nil {
			fmt.Fprintf(&s, "Unavailable: %v\n\n", err)
			continue
		}
		fmt.Fprintf(&s, "%s [%s]\n%s\n\n", path, source, probeTool(ctx, path, spec.args...))
	}
	s.WriteString("Language server recovery (if unavailable):\n" + idetools.RecoveryHint + "\nOverride: SILKIDE_GOPLS must be an absolute executable path.\nAfter changes use Tools > Restart LSP. No background installation or machine PATH changes are performed.")
	return s.String()
}
