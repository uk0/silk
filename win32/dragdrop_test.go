//go:build windows

package win32

import "testing"

// TestDecodePointLMatchesComLayout pins the packing COM actually uses for the
// POINTL that IDropTarget's methods take by value: X in the low half of the
// word, Y in the high half, both signed.
//
// Getting this wrong is not a coordinate bug — reading the point as two
// separate arguments shifts pdwEffect out of the frame entirely, and the
// process dies with an access violation as soon as a drag enters a window.
func TestDecodePointLMatchesComLayout(t *testing.T) {
	cases := []struct{ x, y int32 }{
		{0, 0},
		{1280, 720},
		{-7, 3},
		{3, -7},
		{-2147483648, 2147483647},
	}
	for _, c := range cases {
		packed := uintptr(uint64(uint32(c.y))<<32 | uint64(uint32(c.x)))
		x, y := decodePointL(packed)
		if x != c.x || y != c.y {
			t.Errorf("decodePointL(%#x) = (%d, %d), want (%d, %d)", packed, x, y, c.x, c.y)
		}
	}
}

// TestDropTargetCallbacksHaveValidSignatures relies on syscall.NewCallback
// panicking on any argument that is not machine-word sized: building the vtable
// at package init is itself the check, so reaching this test at all means every
// IDropTarget shim still compiles into a callable stub.
func TestDropTargetCallbacksHaveValidSignatures(t *testing.T) {
	for i, ptr := range []uintptr{
		vtabIDropTarget.QueryInterface, vtabIDropTarget.AddRef, vtabIDropTarget.Release,
		vtabIDropTarget.DragEnter, vtabIDropTarget.DragOver, vtabIDropTarget.DragLeave,
		vtabIDropTarget.Drop,
	} {
		if ptr == 0 {
			t.Errorf("IDropTarget vtable slot %d is null", i)
		}
	}
}
