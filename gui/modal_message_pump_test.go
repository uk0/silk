package gui

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

const (
	testModalEnd  uint32 = 0x8677
	testModalQuit uint32 = 0x12
	testModalWake uint32 = 0x8678
)

func TestBlockingModalPumpServicesPostedWakeAndStopsOnlyOwnEnd(t *testing.T) {
	messages := []modalPumpMessage{{id: testModalWake}, {window: 9, id: 0x102}, {window: 3, id: testModalEnd, wParam: 7}, {window: 8, id: testModalEnd}}
	next, dispatch, service := 0, 0, 0
	var reposted []modalPumpMessage
	pumpBlockingModalMessages(8, testModalEnd, testModalQuit, modalPumpOps{
		next: func() (modalPumpMessage, bool) {
			if next >= len(messages) {
				t.Fatal("modal pump read past its EndModal")
			}
			msg := messages[next]
			next++
			return msg, true
		},
		dispatch: func() { dispatch++ }, service: func() { service++ },
		repost: func(msg modalPumpMessage) { reposted = append(reposted, msg) }, quit: func(int) { t.Fatal("unexpected WM_QUIT") },
	})
	if next != 4 || dispatch != 2 || service != 3 {
		t.Fatalf("reads=%d dispatched=%d services=%d", next, dispatch, service)
	}
	if !reflect.DeepEqual(reposted, []modalPumpMessage{messages[2]}) {
		t.Fatalf("parent EndModal lost: %+v", reposted)
	}
}
func TestBlockingModalPumpPreservesQuitWhenGetMessageReturnsFalse(t *testing.T) {
	quit := -1
	reads := 0
	pumpBlockingModalMessages(1, testModalEnd, testModalQuit, modalPumpOps{
		next: func() (modalPumpMessage, bool) {
			reads++
			return modalPumpMessage{id: testModalQuit, wParam: 42}, false
		},
		dispatch: func() { t.Fatal("WM_QUIT dispatched to a widget") }, service: func() { t.Fatal("service after WM_QUIT") },
		repost: func(modalPumpMessage) { t.Fatal("unexpected deferred end") }, quit: func(code int) { quit = code },
	})
	if quit != 42 || reads != 1 {
		t.Fatalf("quit=%d reads=%d", quit, reads)
	}
}
func TestBlockingModalPumpNestedLoopRetainsParentCompletion(t *testing.T) {
	queue := []modalPumpMessage{{window: 1, id: 0x102}, {window: 1, id: testModalEnd}, {window: 2, id: testModalEnd}}
	var order []string
	next := func() (modalPumpMessage, bool) {
		if len(queue) == 0 {
			t.Fatal("nested modal discarded parent's end message")
		}
		msg := queue[0]
		queue = queue[1:]
		return msg, true
	}
	repost := func(msg modalPumpMessage) { queue = append(queue, msg) }
	service := func() { order = append(order, "service") }
	quit := func(int) { t.Fatal("unexpected quit") }
	pumpBlockingModalMessages(1, testModalEnd, testModalQuit, modalPumpOps{next: next, repost: repost, service: service, quit: quit, dispatch: func() {
		order = append(order, "outer-input")
		pumpBlockingModalMessages(2, testModalEnd, testModalQuit, modalPumpOps{next: next, repost: repost, service: service, quit: quit, dispatch: func() { t.Fatal("end message incorrectly dispatched") }})
		order = append(order, "inner-return")
	}})
	if want := []string{"outer-input", "service", "inner-return", "service"}; !reflect.DeepEqual(order, want) {
		t.Fatalf("order=%v", order)
	}
	if len(queue) != 0 {
		t.Fatalf("queue not drained: %+v", queue)
	}
}
func TestWindowsModalProductionUsesBlockingPump(t *testing.T) {
	// Native calls cannot run on the test goroutine. Pair the pure pump cases
	// with a wiring guard so replacing its production GetMessage by polling is
	// not accidentally 'covered' only by an unused helper.
	src, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func (this *Window) ShowModal(")
	if start < 0 {
		t.Fatal("Windows ShowModal missing")
	}
	end := strings.Index(text[start:], "func (this *Window) EndModal(")
	if end < 0 {
		t.Fatal("Windows modal functions missing")
	}
	body := text[start : start+end]
	if !strings.Contains(body, "ok, err := nextWindowsUIMessage(&msg)") || !strings.Contains(body, "return modalPumpMessage{}, false") || !strings.Contains(body, "pumpBlockingModalMessages(") || !strings.Contains(body, "service: serviceUIFrame") || strings.Contains(body, "win32.PeekMessage(") || strings.Contains(body, "win32.Sleep(") {
		t.Fatal("Windows modal does not use the blocking, UI-servicing production pump")
	}
	picker, err := os.ReadFile("message_fairness_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(picker), "return getUIMessage(msg, 0, 0, 0)") || !strings.Contains(string(picker), "var getUIMessage = win32.GetMessageResult") {
		t.Fatal("fairness picker lost the blocking/error-aware GetMessage fallback")
	}

}

func TestBlockingModalPumpReadFailureStopsWithoutDispatchOrRetry(t *testing.T) {
	reads := 0
	pumpBlockingModalMessages(8, testModalEnd, testModalQuit, modalPumpOps{
		next:     func() (modalPumpMessage, bool) { reads++; return modalPumpMessage{}, false },
		dispatch: func() { t.Fatal("failed GetMessage dispatched stale message") }, service: func() { t.Fatal("failed GetMessage serviced another iteration") },
		repost: func(modalPumpMessage) { t.Fatal("unexpected deferred message") }, quit: func(int) { t.Fatal("failure incorrectly treated as WM_QUIT") },
	})
	if reads != 1 {
		t.Fatalf("failed message retrieval retried %d times", reads)
	}
}
func TestWindowsMainLoopChecksGetMessageErrorBeforeDispatch(t *testing.T) {
	src, err := os.ReadFile("window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	start := strings.Index(text, "func MainLoop() {")
	if start < 0 {
		t.Fatal("MainLoop missing")
	}
	end := strings.Index(text[start:], "\nfunc ")
	if end < 0 {
		t.Fatal("MainLoop boundary missing")
	}
	body := text[start : start+end]
	call := strings.Index(body, "nextWindowsUIMessage(")
	guard := strings.Index(body, "if err != nil {")
	dispatch := strings.Index(body, "win32.DispatchMessage(")
	if call < 0 || guard < call || dispatch < guard || !strings.Contains(body[guard:dispatch], "break") {
		t.Fatal("MainLoop can dispatch/retry after GetMessage failure")
	}
}
