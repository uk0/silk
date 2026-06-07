package core

import (
	"bytes"
	"log"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// captureLog 临时把标准log的输出重定向到buffer, 返回的函数恢复原状并交回所捕获文本
func captureLog(t *testing.T) (restore func() string) {
	t.Helper()
	var buf bytes.Buffer
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	return func() string {
		log.SetOutput(os.Stderr)
		log.SetFlags(oldFlags)
		return buf.String()
	}
}

// TestLogSinkReceivesLevelAndMessage: 调用Warn后, sink应收到LevelWarn及带前缀的文本
func TestLogSinkReceivesLevelAndMessage(t *testing.T) {
	restore := captureLog(t)
	defer restore()

	var gotLevel LogLevel
	var gotMsg string
	unreg := RegisterLogSink(func(level LogLevel, message string) {
		gotLevel = level
		gotMsg = message
	})
	defer unreg()

	Warn("hello ", 42)

	if gotLevel != LevelWarn {
		t.Fatalf("level = %d, want LevelWarn(%d)", gotLevel, LevelWarn)
	}
	if gotMsg != "warning: hello 42" {
		t.Fatalf("msg = %q, want %q", gotMsg, "warning: hello 42")
	}
}

// TestLogSinkLevels: 各日志函数应派发到约定的级别, sink文本与日志输出一致
func TestLogSinkLevels(t *testing.T) {
	restore := captureLog(t)
	defer restore()

	type rec struct {
		level LogLevel
		msg   string
	}
	var got []rec
	unreg := RegisterLogSink(func(level LogLevel, message string) {
		got = append(got, rec{level, message})
	})
	defer unreg()

	Log("a", "b")
	Logf("x=%d", 7)
	Warn("w")
	Error("e")

	want := []rec{
		{LevelInfo, "ab"},
		{LevelInfo, "x=7"},
		{LevelWarn, "warning: w"},
		{LevelError, "error: e"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("record[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestLogSinkMultiple: 注册的多个sink都应收到同一条消息
func TestLogSinkMultiple(t *testing.T) {
	restore := captureLog(t)
	defer restore()

	var a, b string
	unregA := RegisterLogSink(func(_ LogLevel, m string) { a = m })
	defer unregA()
	unregB := RegisterLogSink(func(_ LogLevel, m string) { b = m })
	defer unregB()

	Error("boom")

	if a != "error: boom" || b != "error: boom" {
		t.Fatalf("a=%q b=%q, want both %q", a, b, "error: boom")
	}
}

// TestLogSinkUnregister: 注销某个sink后该sink不再收到消息, 其它sink仍正常
func TestLogSinkUnregister(t *testing.T) {
	restore := captureLog(t)
	defer restore()

	var aCount, bCount int
	unregA := RegisterLogSink(func(_ LogLevel, _ string) { aCount++ })
	unregB := RegisterLogSink(func(_ LogLevel, _ string) { bCount++ })
	defer unregB()

	Warn("first")
	unregA()
	Warn("second")

	if aCount != 1 {
		t.Fatalf("aCount = %d, want 1 (delivery should stop after unregister)", aCount)
	}
	if bCount != 2 {
		t.Fatalf("bCount = %d, want 2 (other sink keeps working)", bCount)
	}
}

// TestLogSinkPanicIsolated: 一个会panic的sink不应影响日志本身或其它sink
func TestLogSinkPanicIsolated(t *testing.T) {
	restore := captureLog(t)
	defer restore()

	var reached bool
	unregBad := RegisterLogSink(func(_ LogLevel, _ string) { panic("sink boom") })
	defer unregBad()
	unregGood := RegisterLogSink(func(_ LogLevel, _ string) { reached = true })
	defer unregGood()

	// 不应panic
	Warn("still works")

	if !reached {
		t.Fatal("good sink was not reached after a panicking sink")
	}
	out := restore()
	if !strings.Contains(out, "warning: still works") {
		t.Fatalf("log output missing expected line, got %q", out)
	}
}

// TestLogSinkReentrant: sink内部再次写日志不应造成死锁
func TestLogSinkReentrant(t *testing.T) {
	restore := captureLog(t)
	defer restore()

	done := make(chan struct{})
	var entered int32
	unreg := RegisterLogSink(func(_ LogLevel, _ string) {
		// 仅在第一次调用时重入写日志一次, 避免无限递归
		if atomic.CompareAndSwapInt32(&entered, 0, 1) {
			Log("reentrant call")
			close(done)
		}
	})
	defer unreg()

	go func() {
		Warn("trigger")
	}()

	select {
	case <-done:
		// 重入调用成功返回, 未死锁
	case <-time.After(2 * time.Second):
		t.Fatal("reentrant sink deadlocked")
	}
}

// TestLogOutputUnchanged: 注册sink不应改变原有的日志输出文本
func TestLogOutputUnchanged(t *testing.T) {
	restore := captureLog(t)
	defer restore()

	unreg := RegisterLogSink(func(_ LogLevel, _ string) {})
	defer unreg()

	Warn("payload ", 1)
	Error("oops")

	out := restore()
	if !strings.Contains(out, "warning: payload 1") {
		t.Fatalf("output missing warning line: %q", out)
	}
	if !strings.Contains(out, "error: oops") {
		t.Fatalf("output missing error line: %q", out)
	}
}
