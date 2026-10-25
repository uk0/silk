// Package formatx formats Go and JSON without launching external tools.
package formatx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/format"
	"unicode/utf8"

	"github.com/uk0/silk/async"
)

const (
	MaxInputBytes  = 8 << 20
	MaxOutputBytes = 16 << 20
	MaxJSONDepth   = 128
)

var (
	ErrLanguage = errors.New("format: unsupported language or operation")
	ErrTooLarge = errors.New("format: document exceeds size limit")
	ErrTooDeep  = errors.New("format: JSON nesting exceeds depth limit")
	ErrUTF8     = errors.New("format: document is not valid UTF-8")
)

// Options.Compact removes JSON whitespace instead of indenting with two spaces.
// Go uses go/format and does not support Compact. Neither mode reorders JSON
// keys, decodes strings/numbers, or silently drops duplicate object members.
type Options struct{ Compact bool }

func validate(language string, size int, options Options) error {
	if language != "go" && language != "json" || language == "go" && options.Compact {
		return ErrLanguage
	}
	if size > MaxInputBytes {
		return ErrTooLarge
	}
	return nil
}

// Format returns a complete formatted document or an error, never partial text.
// JSON pretty output ends in one LF; compact output has no final newline.
// Cancellation is cooperative between bounded phases: the standard Go/JSON
// parser itself cannot be interrupted. Call FormatAsync off the UI thread.
func Format(ctx context.Context, language, text string, options Options) (string, error) {
	if ctx == nil {
		return "", async.ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validate(language, len(text), options); err != nil {
		return "", err
	}
	if !utf8.ValidString(text) {
		return "", ErrUTF8
	}
	var output []byte
	if language == "go" {
		var err error
		output, err = format.Source([]byte(text))
		if err != nil {
			return "", err
		}
	} else {
		// Count structure outside strings before invoking the standard parser.
		if _, err := jsonSize(ctx, text, false); err != nil {
			return "", err
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, []byte(text)); err != nil {
			return "", err
		}
		if options.Compact {
			output = compact.Bytes()
		} else {
			// Exact expansion preflight prevents quadratic indentation allocation.
			size, err := jsonSize(ctx, compact.String(), true)
			if err != nil {
				return "", err
			}
			var pretty bytes.Buffer
			pretty.Grow(size)
			if err := json.Indent(&pretty, compact.Bytes(), "", "  "); err != nil {
				return "", err
			}
			pretty.WriteByte('\n')
			output = pretty.Bytes()
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(output) > MaxOutputBytes {
		return "", ErrTooLarge
	}
	return string(output), nil
}

// jsonSize validates resource bounds (not JSON syntax). With pretty=true the
// input is valid compact JSON and the count includes a final LF.
func jsonSize(ctx context.Context, text string, pretty bool) (int, error) {
	depth, size := 0, len(text)+1
	inString, escaped := false, false
	for i := 0; i < len(text); i++ {
		if i&4095 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
		}
		c := text[i]
		if inString {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > MaxJSONDepth {
				return 0, ErrTooDeep
			}
			if pretty && i+1 < len(text) && text[i+1] != '}' && text[i+1] != ']' {
				size += 1 + 2*depth
			}
		case '}', ']':
			depth--
			if pretty && i > 0 && text[i-1] != '{' && text[i-1] != '[' {
				size += 1 + 2*depth
			}
		case ',':
			if pretty {
				size += 1 + 2*depth
			}
		case ':':
			if pretty {
				size++
			}
		}
		if pretty && size > MaxOutputBytes {
			return 0, ErrTooLarge
		}
	}
	return size, ctx.Err()
}

var formatExecutor = async.NewExecutor(2)

// FormatAsync snapshots immutable strings and options. A nil executor uses a
// shared two-job executor; overload returns async.ErrBusy without a goroutine.
// Completion delivery is explicitly selected through Future.OnComplete.
func FormatAsync(ctx context.Context, executor *async.Executor, language, text string, options Options) (*async.Future[string], error) {
	if err := validate(language, len(text), options); err != nil {
		return nil, err
	}
	if executor == nil {
		executor = formatExecutor
	}
	return async.Submit(ctx, executor, func(ctx context.Context) (string, error) { return Format(ctx, language, text, options) })
}
