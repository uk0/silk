package win32

import (
	"testing"
	"unsafe"
)

// IMM structures contain only DWORD/LONG, not native pointers, on all targets.
func TestInputMethodStructureABI(t *testing.T) {
	if n := unsafe.Sizeof(COMPOSITIONFORM{}); n != 28 {
		t.Fatalf("COMPOSITIONFORM size=%d", n)
	}
	if n := unsafe.Sizeof(CANDIDATEFORM{}); n != 32 {
		t.Fatalf("CANDIDATEFORM size=%d", n)
	}
	if n := unsafe.Offsetof(COMPOSITIONFORM{}.RcArea); n != 12 {
		t.Fatalf("COMPOSITIONFORM area=%d", n)
	}
	if n := unsafe.Offsetof(CANDIDATEFORM{}.RcArea); n != 16 {
		t.Fatalf("CANDIDATEFORM area=%d", n)
	}
}
