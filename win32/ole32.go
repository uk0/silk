//go:build windows

// Copyright 2010-2012 The W32 Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package win32

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	modole32            = syscall.NewLazyDLL("ole32.dll")
	procOleInitialize   = modole32.NewProc("OleInitialize")
	procOleUninitialize = modole32.NewProc("OleUninitialize")
	//procCoInitializeEx        = modole32.NewProc("CoInitializeEx")
	//procCoInitialize          = modole32.NewProc("CoInitialize")
	//procCoUninitialize        = modole32.NewProc("CoUninitialize")
	procCreateStreamOnHGlobal = modole32.NewProc("CreateStreamOnHGlobal")
	procRevokeDragDrop        = modole32.NewProc("RevokeDragDrop")
	procRegisterDragDrop      = modole32.NewProc("RegisterDragDrop")
	procIsEqualGUID           = modole32.NewProc("IsEqualGUID")
	procDoDragDrop            = modole32.NewProc("DoDragDrop")
	procCoTaskMemFree         = modole32.NewProc("CoTaskMemFree")
	oleInitialized            bool
)

func OleInitialize() HRESULT {
	if oleInitialized {
		return S_OK
	}
	ret, _, _ := procOleInitialize.Call(0)
	if ret == S_OK {
		oleInitialized = true
	} else {
		panic("OleInitialize failed with " + HRESULT(ret).String())
	}
	return HRESULT(ret)
}

func OleUninitialize() {
	procOleUninitialize.Call()
}

//func CoInitializeEx(coInit uintptr) HRESULT {
//	ret, _, _ := procCoInitializeEx.Call(
//		0,
//		coInit)

//	switch uint32(ret) {
//	case E_INVALIDARG:
//		panic("CoInitializeEx failed with E_INVALIDARG")
//	case E_OUTOFMEMORY:
//		panic("CoInitializeEx failed with E_OUTOFMEMORY")
//	case E_UNEXPECTED:
//		panic("CoInitializeEx failed with E_UNEXPECTED")
//	}

//	return HRESULT(ret)
//}

//func CoInitialize() {
//	procCoInitialize.Call(0)
//}

//func CoUninitialize() {
//	procCoUninitialize.Call()
//}

func CreateStreamOnHGlobal(hGlobal HGLOBAL, fDeleteOnRelease bool) *IStream {
	stream := new(IStream)
	ret, _, _ := procCreateStreamOnHGlobal.Call(
		uintptr(hGlobal),
		uintptr(BoolToBOOL(fDeleteOnRelease)),
		uintptr(unsafe.Pointer(&stream)))

	switch uint32(ret) {
	case E_INVALIDARG:
		panic("CreateStreamOnHGlobal failed with E_INVALIDARG")
	case E_OUTOFMEMORY:
		panic("CreateStreamOnHGlobal failed with E_OUTOFMEMORY")
	case E_UNEXPECTED:
		panic("CreateStreamOnHGlobal failed with E_UNEXPECTED")
	}

	return stream
}

func IsEqualGUID(a, b *GUID) bool {
	ret, _, _ := procIsEqualGUID.Call(
		uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(b)))
	return ret != 0
}

func DoDragDrop(pDataObj *DataObject, pDropSource *DropSource,
	dwOKEffects uint32) (dwEffec uint32, err error) {
	ret, _, _ := procDoDragDrop.Call(
		uintptr(unsafe.Pointer(pDataObj)),
		uintptr(unsafe.Pointer(pDropSource)),
		uintptr(dwOKEffects),
		uintptr(unsafe.Pointer(&dwEffec)))
	if ret&0x80000000 == 0 {
		return
	}
	err = errors.New(HRESULT(ret).String())
	return
}

func CoTaskMemFree(p unsafe.Pointer) {
	procCoTaskMemFree.Call(uintptr(p))
}
