package win32

import (
	"syscall"
	"unsafe"
)

type HIMC HANDLE

type COMPOSITIONFORM struct {
	DwStyle      uint32
	PtCurrentPos POINT
	RcArea       RECT
}

type CANDIDATEFORM struct {
	DwIndex      uint32
	DwStyle      uint32
	PtCurrentPos POINT
	RcArea       RECT
}

const (
	CFS_POINT           = 0x0002
	CFS_EXCLUDE         = 0x0080
	IMN_CHANGECANDIDATE = 0x0003
	IMN_OPENCANDIDATE   = 0x0005
	UNICODE_NOCHAR      = 0xFFFF
)

var (
	modimm32                    = syscall.NewLazyDLL("imm32.dll")
	procImmGetContext           = modimm32.NewProc("ImmGetContext")
	procImmReleaseContext       = modimm32.NewProc("ImmReleaseContext")
	procImmSetCompositionWindow = modimm32.NewProc("ImmSetCompositionWindow")
	procImmSetCandidateWindow   = modimm32.NewProc("ImmSetCandidateWindow")
)

// The returned context is borrowed and must be paired with ImmReleaseContext.
func ImmGetContext(hwnd HWND) HIMC {
	r, _, _ := procImmGetContext.Call(uintptr(hwnd))
	return HIMC(r)
}

func ImmReleaseContext(hwnd HWND, context HIMC) bool {
	r, _, _ := procImmReleaseContext.Call(uintptr(hwnd), uintptr(context))
	return r != 0
}

func ImmSetCompositionWindow(context HIMC, form *COMPOSITIONFORM) bool {
	r, _, _ := procImmSetCompositionWindow.Call(uintptr(context), uintptr(unsafe.Pointer(form)))
	return r != 0
}

func ImmSetCandidateWindow(context HIMC, form *CANDIDATEFORM) bool {
	r, _, _ := procImmSetCandidateWindow.Call(uintptr(context), uintptr(unsafe.Pointer(form)))
	return r != 0
}
