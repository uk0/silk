package main

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// streamThrottle is the minimum gap between progress callbacks. A cgo build of
// this repo emits thousands of lines; repainting (and re-parsing for the
// Problems pane) on every one of them would cost more than the build.
const streamThrottle = 200 * time.Millisecond

// streamCommand runs cmd and reports its combined output to onProgress AS IT
// ARRIVES, returning the full text and the exit error.
//
// It replaces cmd.CombinedOutput() on the toolchain paths. CombinedOutput
// blocks until the process exits and yields nothing in the meantime, so a
// first-time cgo build on Windows — cairo plus the multi-megabyte SQLite
// amalgamation across ~170 packages, minutes of work — looked to the user
// exactly like a frozen IDE, and the usual reaction is to kill it (which then
// leaves the run lock behind and makes the next start report an abnormal exit).
//
// stdout and stderr are drained by separate goroutines: reading them in
// sequence deadlocks as soon as the child fills the pipe the reader is not
// looking at, which Windows' smaller pipe buffers hit sooner.
//
// onProgress receives the accumulated text (not the delta) because the output
// panes take a whole document; it is called from a worker goroutine, so a UI
// caller must marshal it. It is throttled to streamThrottle and always fires
// once more at the end so the final text is never lost.
func streamCommand(cmd *exec.Cmd, onProgress func(text string)) (string, error) {
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	var mu sync.Mutex
	var buf strings.Builder
	last := time.Time{}

	emit := func(force bool) {
		mu.Lock()
		text := buf.String()
		due := force || time.Since(last) >= streamThrottle
		if due {
			last = time.Now()
		}
		mu.Unlock()
		if due && onProgress != nil {
			onProgress(text)
		}
	}

	var wg sync.WaitGroup
	drain := func(r io.Reader) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		// Build errors can carry very long lines (a full type mismatch);
		// the 64K default would truncate them.
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for sc.Scan() {
			mu.Lock()
			buf.WriteString(sc.Text())
			buf.WriteByte('\n')
			mu.Unlock()
			emit(false)
		}
	}
	wg.Add(2)
	go drain(stdout)
	go drain(stderr)
	wg.Wait()

	waitErr := cmd.Wait()
	emit(true)

	mu.Lock()
	text := buf.String()
	mu.Unlock()
	return text, waitErr
}

// runWithProgress is the toolchain-command wrapper the IDE actions use: it
// streams output under a fixed header and appends a live elapsed-seconds
// heartbeat, so a long build is visibly working instead of looking hung.
// report is called on the UI thread by the caller's onUI wrapper.
func runWithProgress(cmd *exec.Cmd, header string, report func(text string)) (string, error) {
	start := time.Now()
	done := make(chan struct{})

	var mu sync.Mutex
	latest := ""

	show := func(tail string) {
		mu.Lock()
		body := latest
		mu.Unlock()
		report(header + "\n" + body + tail)
	}

	// Heartbeat: even a build that prints nothing for a minute now shows a
	// ticking elapsed time.
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				show(fmt.Sprintf("\n... running %ds", int(time.Since(start).Seconds())))
			}
		}
	}()

	text, err := streamCommand(cmd, func(sofar string) {
		mu.Lock()
		latest = sofar
		mu.Unlock()
		show("")
	})
	close(done)

	mu.Lock()
	latest = text
	mu.Unlock()
	return text, err
}
