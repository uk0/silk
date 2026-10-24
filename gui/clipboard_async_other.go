//go:build !darwin

package gui

import "context"

const asyncClipboardControls = false

func readClipboardTextAsync(ctx context.Context, done func(string, error)) error {
	Post(func() {
		if err := ctx.Err(); err != nil {
			done("", err)
			return
		}
		data, err := Clipboard.Data("text/plain")
		text, _ := data.(string)
		if ctx.Err() != nil {
			done("", ctx.Err())
			return
		}
		if len(text) > maxClipboardTextBytes {
			done("", ErrClipboardTooLarge)
			return
		}
		done(text, err)
	})
	return nil
}
