package win32

import (
	"testing"
	"unsafe"
)

func TestDropSourceCancellationAndIdentity(t *testing.T) {
	source := NewDropSource(nil)
	for _, tc := range []struct {
		name   string
		escape int32
		keys   uint32
		want   HRESULT
	}{
		{"holding", 0, MK_LBUTTON, S_OK},
		{"release", 0, 0, DRAGDROP_S_DROP},
		{"escape-held", 1, MK_LBUTTON, DRAGDROP_S_CANCEL},
		{"escape-release", 1, 0, DRAGDROP_S_CANCEL},
		{"escape-bool", -1, MK_LBUTTON | MK_CONTROL, DRAGDROP_S_CANCEL},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := source.queryContinueDrag(tc.escape, tc.keys); got != tc.want {
				t.Fatalf("status = %v, want %v", got, tc.want)
			}
		})
	}
	for _, tc := range []struct {
		name     string
		id       *GUID
		accepted bool
	}{
		{"unknown", IID_IUnknown, true}, {"source", IID_IDropSource, true}, {"not-target", IID_IDropTarget, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var object unsafe.Pointer = unsafe.Pointer(source)
			got := source.queryInterface(tc.id, &object)
			if tc.accepted {
				if got != S_OK || object != unsafe.Pointer(source) {
					t.Fatalf("wrong source identity: %v, %p", got, object)
				}
				if source.release() != 0 {
					t.Fatal("query reference not balanced")
				}
			} else {
				if got == S_OK {
					source.release()
				}
				if got != E_NOINTERFACE || object != nil {
					t.Fatalf("source advertises incompatible target vtable: %v, %p", got, object)
				}
			}
		})
	}
}
