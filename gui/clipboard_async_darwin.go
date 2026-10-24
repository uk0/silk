package gui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"time"

	"github.com/uk0/silk/async"
)

const asyncClipboardControls = true

var clipboardReadExecutor = async.NewExecutor(2)

// GLFW clipboard access is main-thread-only. pbpaste is an OS-owned helper:
// AppKit/pasteboard provider stalls block its process, not the IDE's UI thread.
// Never invoke a shell or PATH-resolved executable, and bound time/output.
func readClipboardTextAsync(ctx context.Context, done func(string, error)) error {
	if getAnyGLFWWindow() == nil {
		Post(func() { done("", errNoClipboardWindow) })
		return nil
	}
	f, err := async.Submit(ctx, clipboardReadExecutor, func(ctx context.Context) (string, error) {
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		return readMacClipboardText(ctx)
	})
	if err != nil {
		return err
	}
	return f.OnComplete(Post, func(text string, err error) {
		if ctx.Err() != nil {
			done("", ctx.Err())
			return
		}
		done(text, err)
	})
}

type clipboardOutput struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *clipboardOutput) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.buffer.Len() {
		b.overflow = true
		return 0, ErrClipboardTooLarge
	}
	return b.buffer.Write(p)
}
func readMacClipboardText(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "/usr/bin/pbpaste", "-Prefer", "txt")
	return readClipboardCommand(ctx, cmd)
}
func readClipboardCommand(ctx context.Context, cmd *exec.Cmd) (string, error) {
	var out = clipboardOutput{limit: maxClipboardTextBytes}
	cmd.Stdout = &out
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 250 * time.Millisecond
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if out.overflow || errors.Is(err, ErrClipboardTooLarge) {
		return "", ErrClipboardTooLarge
	}
	if err != nil {
		return "", err
	}
	text := out.buffer.String()
	if err := clipboardTextErr(text); err != nil {
		return "", err
	}
	return text, nil
}
