package gui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestClipboardProcessHelper(t *testing.T) {
	mode := os.Getenv("SILK_CLIPBOARD_PROCESS_FIXTURE")
	if mode == "" {
		return
	}
	switch mode {
	case "text":
		fmt.Print("中文\r\n😀\n")
	case "large":
		fmt.Print(strings.Repeat("x", maxClipboardTextBytes+1))
	case "nul":
		fmt.Print("a\x00b")
	case "sleep":
		time.Sleep(10 * time.Second)
	case "failure":
		os.Exit(7)
	}
	os.Exit(0)
}
func TestClipboardHelperBoundedExactAndCancellable(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		mode, want string
		wantErr    error
	}{
		{"text", "中文\r\n😀\n", nil}, {"large", "", ErrClipboardTooLarge}, {"nul", "", errClipboardTextNUL}, {"sleep", "", context.DeadlineExceeded},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			duration := 5 * time.Second
			if tt.mode == "sleep" {
				duration = 300 * time.Millisecond
			}
			ctx, cancel := context.WithTimeout(context.Background(), duration)
			defer cancel()
			cmd := exec.CommandContext(ctx, exe, "-test.run=^TestClipboardProcessHelper$")
			cmd.Env = append(os.Environ(), "SILK_CLIPBOARD_PROCESS_FIXTURE="+tt.mode)
			got, err := readClipboardCommand(ctx, cmd)
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Fatalf("mode=%s got length=%d err=%v", tt.mode, len(got), err)
			}
		})
	}
	t.Run("failure", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestClipboardProcessHelper$")
		cmd.Env = append(os.Environ(), "SILK_CLIPBOARD_PROCESS_FIXTURE=failure")
		got, err := readClipboardCommand(ctx, cmd)
		var exitErr *exec.ExitError
		if got != "" || !errors.As(err, &exitErr) || exitErr.ExitCode() != 7 {
			t.Fatalf("helper failure returned %q, %v", got, err)
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readMacClipboardText(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
