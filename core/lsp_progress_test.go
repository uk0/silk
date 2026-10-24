package core

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLSPProgressFloodCannotDisplaceDiagnostics(t *testing.T) {
	c, in, out := newPipedClient(t)
	defer in.Close()
	defer out.Close()
	updates := c.WorkProgressUpdates()
	barrier := make(chan *LSPMessage, 1)
	c.mu.Lock()
	c.pending[91] = barrier
	c.mu.Unlock()
	go func() {
		write := func(m *LSPMessage) { _ = WriteLSPMessage(out, m) }
		write(&LSPMessage{JSONRPC: "2.0", Method: "$/progress", Params: json.RawMessage(`{"token":1,"value":{"kind":"begin","title":"Indexing"}}`)})
		for i := 0; i < 1000; i++ {
			write(&LSPMessage{JSONRPC: "2.0", Method: "$/progress", Params: json.RawMessage(fmt.Sprintf(`{"token":1,"value":{"kind":"report","message":"%d"}}`, i))})
		}
		write(&LSPMessage{JSONRPC: "2.0", Method: "textDocument/publishDiagnostics", Params: json.RawMessage(`{"uri":"file:///a.go","diagnostics":[]}`)})
		id := json.RawMessage(`91`)
		write(&LSPMessage{JSONRPC: "2.0", ID: &id, Result: json.RawMessage(`null`)})
	}()
	select {
	case <-barrier:
	case <-time.After(5 * time.Second):
		t.Fatal("progress blocked the RPC read loop")
	}
	if n := len(c.notifications); n != 1 {
		t.Fatalf("progress displaced diagnostics: queue has %d messages, want only the diagnostic", n)
	}
	if m := <-c.notifications; m.Method != "textDocument/publishDiagnostics" {
		t.Fatal("diagnostic notification lost")
	}
	if len(updates) != 1 || c.WorkProgressSnapshot().Active[0].Message != "999" {
		t.Fatal("progress burst did not retain one latest-state wake")
	}
	_ = out.Close()
	select {
	case <-c.done:
	case <-time.After(time.Second):
		t.Fatal("progress reader leaked")
	}
	for range updates {
	} // includes an already queued wake, then closes
	if s := c.WorkProgressSnapshot(); !s.Closed || len(s.Active) != 0 {
		t.Fatal(s)
	}
}

func feedWork(c *LSPClient, s string) bool {
	return c.routeWorkProgress(&LSPMessage{Method: "$/progress", Params: json.RawMessage(s)})
}

func TestLSPProgressSnapshotOwnershipAndOptionalFields(t *testing.T) {
	c := &LSPClient{}
	ch := c.WorkProgressUpdates()
	feedWork(c, `{"token":7,"value":{"kind":"begin","title":"Numeric","message":"initial","percentage":0,"cancellable":true}}`)
	feedWork(c, `{"token":"7","value":{"kind":"begin","title":"String"}}`)
	feedWork(c, `{"token":7,"value":{"kind":"report","message":"next","percentage":50}}`)
	s := c.WorkProgressSnapshot()
	if len(s.Active) != 2 || s.Active[0].Title != "Numeric" || s.Active[1].Title != "String" || !s.Active[0].Cancellable || *s.Active[0].Percentage != 50 {
		t.Fatal(s)
	}
	s.Active[0].Token[0] = '9'
	*s.Active[0].Percentage = 99
	if s = c.WorkProgressSnapshot(); string(s.Active[0].Token) != "7" || *s.Active[0].Percentage != 50 {
		t.Fatal("snapshot aliases transport state")
	}
	<-ch
	feedWork(c, `{"token":7,"value":{"kind":"report","message":"next","percentage":50}}`)
	if len(ch) != 0 {
		t.Fatal("identical report scheduled another update")
	}
	feedWork(c, `{"token":7,"value":{"kind":"report","message":"","cancellable":false}}`)
	s = c.WorkProgressSnapshot()
	if s.Active[0].Message != "" || s.Active[0].Cancellable || *s.Active[0].Percentage != 50 {
		t.Fatal("optional report fields lost state")
	}
	feedWork(c, `{"token":7,"value":{"kind":"end","message":"done"}}`)
	feedWork(c, `{"token":7,"value":{"kind":"report","message":"late"}}`)
	s = c.WorkProgressSnapshot()
	if len(s.Active) != 1 || s.LastCompleted != "Numeric" || string(s.Active[0].Token) != `"7"` {
		t.Fatal(s)
	}
}

func TestLSPProgressLimitsAndMalformedValues(t *testing.T) {
	c := &LSPClient{}
	c.WorkProgressUpdates()
	for _, raw := range []string{`{"token":{},"value":{"kind":"begin","title":"bad"}}`, `{"token":1,"value":{"kind":"begin","title":"bad","percentage":101}}`, `{"token":1,"value":{"kind":"report","message":"orphan"}}`, `{"token":1,"value":{"kind":"end"}}`} {
		feedWork(c, raw)
	}
	if len(c.WorkProgressSnapshot().Active) != 0 {
		t.Fatal("malformed/orphan progress retained")
	}
	for i := 0; i < 80; i++ {
		feedWork(c, fmt.Sprintf(`{"token":%d,"value":{"kind":"begin","title":%q,"message":%q}}`, i, strings.Repeat("中", 300), strings.Repeat("😀", 300)))
	}
	s := c.WorkProgressSnapshot()
	if len(s.Active) != 64 || !s.Limited || len(s.Active[0].Title) > 256 || len(s.Active[0].Message) > 512 {
		t.Fatal("progress retention unbounded")
	}
	for i := 0; i < 80; i++ {
		feedWork(c, fmt.Sprintf(`{"token":%d,"value":{"kind":"end"}}`, i))
	}
	if s = c.WorkProgressSnapshot(); len(s.Active) != 0 || !s.Limited {
		t.Fatal("incomplete view became a false idle claim")
	}
	d := &LSPClient{}
	d.WorkProgressUpdates()
	feedWork(d, fmt.Sprintf(`{"token":1,"value":{"kind":"begin","title":%q}}`, strings.Repeat("x", 20000)))
	if s = d.WorkProgressSnapshot(); !s.Limited || len(s.Active) != 0 {
		t.Fatal("oversized work progress retained")
	}
	feedWork(d, `{"token":2,"value":{"kind":"begin","title":"Finish me"}}`)
	feedWork(d, fmt.Sprintf(`{"token":2,"value":{"kind":"end","message":%q}}`, strings.Repeat("x", 20000)))
	if s = d.WorkProgressSnapshot(); len(s.Active) != 0 || s.LastCompleted != "Finish me" {
		t.Fatal("oversized final message left a completed job stuck")
	}
}

func TestLSPProgressOptInAndPartialResults(t *testing.T) {
	c := &LSPClient{}
	if feedWork(c, `{"token":1,"value":{"kind":"begin","title":"unadvertised"}}`) {
		t.Fatal("default host notifications intercepted")
	}
	c.WorkProgressUpdates()
	for _, raw := range []string{`{"token":"symbols","value":[{"name":"x"}]}`, `{"token":4,"value":{"items":[]}}`, `{"token":4,"value":{"kind":"custom"}}`} {
		if feedWork(c, raw) {
			t.Fatal("partial/unknown progress swallowed")
		}
	}
	c.closeWorkProgress()
	c.closeWorkProgress()
	if _, ok := <-c.WorkProgressUpdates(); ok {
		t.Fatal("closed transport reopened progress lane")
	}
	d := &LSPClient{}
	d.closeWorkProgress()
	if _, ok := <-d.WorkProgressUpdates(); ok {
		t.Fatal("late opt-in channel not closed")
	}
}

func TestLSPProgressConcurrentSnapshots(t *testing.T) {
	c := &LSPClient{}
	c.WorkProgressUpdates()
	feedWork(c, `{"token":"job","value":{"kind":"begin","title":"Indexing","percentage":0}}`)
	var wg sync.WaitGroup
	for n := 0; n < 4; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 300; i++ {
				feedWork(c, fmt.Sprintf(`{"token":"job","value":{"kind":"report","percentage":%d}}`, i%101))
				s := c.WorkProgressSnapshot()
				if len(s.Active) > 0 {
					s.Active[0].Token[0] = '!'
					*s.Active[0].Percentage = 999
				}
			}
		}()
	}
	wg.Wait()
	if s := c.WorkProgressSnapshot(); *s.Active[0].Percentage > 100 || string(s.Active[0].Token) != `"job"` {
		t.Fatal("caller mutated shared progress")
	}
}
