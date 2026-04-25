package win32

/*
//
// typedef  void* (__stdcall *FUNC_IUnknown_addref)(void* this);
// __stdcall void* IUnknown_addref(void* this, void*callback) {
//     return ((FUNC_IUnknown_addref)callback)(this);
// }
// typedef  void* (__stdcall *FUNC_DataObject_getData)(void* this, void* fetc, void * medium);
// __stdcall void* DataObject_getData(void* this, void* fetc, void * medium, void*callback) {
//     return ((FUNC_DataObject_getData)callback)(this, fetc, medium);
// }
// typedef  void* (__stdcall *FUNC_DataObject_enumFormatEtc)(void* this, unsigned long dwDirection, void** ppEnumFormatEtc);
// __stdcall void* DataObject_enumFormatEtc(void* this, unsigned long dwDirection, void** ppEnumFormatEtc, void*callback) {
//     return ((FUNC_DataObject_enumFormatEtc)callback)(this, dwDirection, ppEnumFormatEtc);
// }
// typedef  void* (__stdcall *FUNC_IEnumFORMATETC_Next)(void* this, unsigned long celt, void* rgelt, unsigned long * pceltFetched);
// __stdcall void* IEnumFORMATETC_Next(void* this, unsigned long celt, void* rgelt, unsigned long * pceltFetched, void*callback) {
//     return ((FUNC_IEnumFORMATETC_Next)callback)(this,  celt, rgelt, pceltFetched);
// }
import "C"
*/

import (
	//"silk/core"
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"
)

var comRefMap = make(map[unsafe.Pointer]int32)
var comMutex sync.Mutex

func lockInc(p unsafe.Pointer) int32 {
	comMutex.Lock()
	defer comMutex.Unlock()
	n := comRefMap[p]
	n++
	comRefMap[p] = n
	return n
}

func lockDec(p unsafe.Pointer) int32 {
	comMutex.Lock()
	defer comMutex.Unlock()
	n := comRefMap[p]
	n--
	if n <= 0 {
		delete(comRefMap, p)
	} else {
		comRefMap[p] = n
	}
	return n
}

type DVTARGETDEVICE struct {
	tdSize             uint32
	tdDriverNameOffset uint16
	tdDeviceNameOffset uint16
	tdPortNameOffset   uint16
	tdExtDevmodeOffset uint16
	tdData             [1]byte
}

type FORMATETC struct {
	cfFormat CLIPFORMAT
	ptd      *DVTARGETDEVICE
	dwAspect uint32
	lindex   int32
	tymed    uint32
}

// 以下接口暂不支持
type IAdviseSink uintptr
type IEnumSTATDATA uintptr

type IDataObjectVtbl struct {
	QueryInterface        uintptr
	AddRef                uintptr
	Release               uintptr
	GetData               uintptr
	GetDataHere           uintptr
	QueryGetData          uintptr
	GetCanonicalFormatEtc uintptr
	SetData               uintptr
	EnumFormatEtc         uintptr
	DAdvise               uintptr
	DUnadvise             uintptr
	EnumDAdvise           uintptr
}

/*
typedef struct tagSTGMEDIUM {
  DWORD    tymed;
  union {
    HBITMAP       hBitmap;
    HMETAFILEPICT hMetaFilePict;
    HENHMETAFILE  hEnhMetaFile;
    HGLOBAL       hGlobal;
    LPOLESTR      lpszFileName;
    IStream       *pstm;
    IStorage      *pstg;
  };
  IUnknown *pUnkForRelease;
} STGMEDIUM, *LPSTGMEDIUM;
*/

type STGMEDIUM struct {
	TyMed          uint32
	Data           unsafe.Pointer
	PUnkForRelease *IUnknown
}

func (p *STGMEDIUM) destroy() {
	if p.PUnkForRelease != nil {
		p.PUnkForRelease.Release()
		return
	}
	switch p.TyMed {
	case TYMED_HGLOBAL:
		GlobalFree(HGLOBAL(p.Data))
	case TYMED_FILE:
		CoTaskMemFree(p.Data)
	case TYMED_ISTREAM:
	case TYMED_ISTORAGE:
	case TYMED_GDI:
	case TYMED_MFPICT:
	case TYMED_ENHMF:
	case TYMED_NULL:
		panic("Unsupported medium type.")
	}
}

func (p *STGMEDIUM) HGlobal() HGLOBAL {
	return HGLOBAL(p.Data)
}

func (p *STGMEDIUM) File() string {
	//return HGLOBAL(p.Data)
	return ""
}

type IDataObject struct {
	vtab *IDataObjectVtbl
}

type IEnumFORMATETCVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	Next           uintptr
	Skip           uintptr
	Reset          uintptr
	Clone          uintptr
}

type IEnumFORMATETC struct {
	vtab *IEnumFORMATETCVtbl
}

//func (this *IEnumFORMATETC) AddRef() int {
//	p := C.IUnknown_addref(unsafe.Pointer(this), unsafe.Pointer(this.vtab.AddRef))
//	return int(uintptr(p))
//}

func (this *IEnumFORMATETC) Release() int32 {
	return ComRelease((*IUnknown)(unsafe.Pointer(this)))
}

//func (this *IEnumFORMATETC) Next(celt uint32, rgelt *FORMATETC, pceltFetched *uint32) HRESULT {

//}

func (this *IDataObject) QueryInterface(id *GUID) *IDispatch {
	return ComQueryInterface((*IUnknown)(unsafe.Pointer(this)), id)
}

func (this *IDataObject) AddRef() int32 {
	return ComAddRef((*IUnknown)(unsafe.Pointer(this)))
}

func (this *IDataObject) Release() int32 {
	return ComRelease((*IUnknown)(unsafe.Pointer(this)))
}

func (this *IDataObject) getData(fetc *FORMATETC, medium *STGMEDIUM) HRESULT {
	ret, _, _ := syscall.Syscall(this.vtab.GetData, 3,
		uintptr(unsafe.Pointer(this)),
		uintptr(unsafe.Pointer(fetc)),
		uintptr(unsafe.Pointer(medium)))
	return HRESULT(ret)
}

func (this *IDataObject) enumFormatEtc(dwDirection uint32, ppEnumFormatEtc **IEnumFORMATETC) HRESULT {
	ret, _, _ := syscall.Syscall(this.vtab.EnumFormatEtc, 3,
		uintptr(unsafe.Pointer(this)),
		uintptr(dwDirection),
		uintptr(unsafe.Pointer(ppEnumFormatEtc)))
	return HRESULT(ret)
}

func (this *IDataObject) Formats() (ret []CLIPFORMAT) {
	var pEnumFormatEtc *IEnumFORMATETC
	hr := this.enumFormatEtc(1, &pEnumFormatEtc)
	if hr != S_OK {
		return
	}

	var celt, pceltFetched uint32
	var rgelt [1024]FORMATETC
	hr1, _, _ := syscall.Syscall6(pEnumFormatEtc.vtab.Next, 4,
		uintptr(unsafe.Pointer(pEnumFormatEtc)),
		uintptr(celt),
		uintptr(unsafe.Pointer(&rgelt[0])),
		uintptr(unsafe.Pointer(&pceltFetched)),
		0,
		0)
	if hr1 == S_OK {
		for i := 0; i < int(pceltFetched); i++ {
			ret = append(ret, rgelt[i].cfFormat)
			if rgelt[i].ptd != nil {
				CoTaskMemFree(unsafe.Pointer(rgelt[i].ptd))
			}
		}
	}
	pEnumFormatEtc.Release()

	return ret
}

func (this *IDataObject) Data(cf CLIPFORMAT) *STGMEDIUM {
	fetc := FORMATETC{}
	fetc.cfFormat = cf
	fetc.dwAspect = 1
	fetc.lindex = -1
	fetc.tymed = TYMED_HGLOBAL | TYMED_FILE

	medium := new(STGMEDIUM)
	hr := this.getData(&fetc, medium)
	if hr == S_OK {
		return medium
		runtime.SetFinalizer(medium, (*STGMEDIUM).destroy)
		return medium
	}
	return nil
}

func (this *IDataObject) HasFormat(cf CLIPFORMAT) bool {
	fetc := FORMATETC{}
	fetc.cfFormat = cf
	fetc.dwAspect = 1
	fetc.lindex = -1
	fetc.tymed = TYMED_HGLOBAL | TYMED_FILE

	hr, _, _ := syscall.Syscall(this.vtab.QueryGetData, 2,
		uintptr(unsafe.Pointer(this)),
		uintptr(unsafe.Pointer(&fetc)),
		0)
	return hr == S_OK
}

type DataObject struct {
	vtab       *IDataObjectVtbl
	data       []HGLOBAL
	formatEtcs []FORMATETC
	from       interface{}
}

func (this *DataObject) queryInterface(riid *GUID, ppvObject *unsafe.Pointer) HRESULT {
	//return ComQueryInterface((*IUnknown)(unsafe.Pointer(this)), id)
	*ppvObject = nil
	if IsEqualGUID(riid, IID_IUnknown) {
		this.addRef()
		*ppvObject = unsafe.Pointer(this)
		return S_OK
	}
	if IsEqualGUID(riid, IID_IDataObject) {
		this.addRef()
		*ppvObject = unsafe.Pointer(this)
		return S_OK
	}
	if IsEqualGUID(riid, IID_OurDataObject) {
		this.addRef()
		*ppvObject = unsafe.Pointer(this)
		return S_OK
	}
	return E_NOINTERFACE
}

func (this *DataObject) addRef() HRESULT {
	return HRESULT(lockInc(unsafe.Pointer(this)))
}

func (this *DataObject) release() HRESULT {
	return HRESULT(lockDec(unsafe.Pointer(this)))
}

func (this *DataObject) lookupFormatEtc(p *FORMATETC) int {
	for i, _ := range this.data {
		v := &this.formatEtcs[i]
		if (p.tymed&v.tymed != 0) &&
			p.cfFormat == v.cfFormat &&
			p.dwAspect == v.dwAspect {
			//core.Debug(i, " -> ", v.cfFormat.String(), " ", *v)
			return i
		}
	}
	//	panic("lookupFormatEtc")
	return -1
}

func (this *DataObject) getData(fetc *FORMATETC, medium *STGMEDIUM) HRESULT {
	index := this.lookupFormatEtc(fetc)
	if index == -1 {
		return DV_E_FORMATETC
	}
	//	core.Debug("aaaaaaaaaaaaaaaaaaaa")
	medium.TyMed = TYMED_HGLOBAL
	medium.PUnkForRelease = nil
	nLen := GlobalSize(this.data[index])
	if nLen == 0 {
		return STG_E_MEDIUMFULL
	}
	//	core.Debug("bbbbbbbbbbbbbbbbbbbbbbb")
	hGlobal := GlobalAlloc(GMEM_FIXED, uint32(nLen))
	if hGlobal == 0 {
		return STG_E_MEDIUMFULL
	}

	//	core.Debug("ccccccccccccccccccccccccc")
	pData := GlobalLock(this.data[index])
	if pData == nil {
		GlobalFree(hGlobal)
		return STG_E_MEDIUMFULL
	}

	//	core.Debug("dddddddddddddddddddddddddddddddd")
	src := (*((*[1 << 30]byte)(pData)))[:nLen]
	dst := (*((*[1 << 30]byte)(unsafe.Pointer(hGlobal))))[:nLen]
	copy(dst, src)

	//	core.Debug("eeeeeeeeeeeeeeeeeeeeeee")
	GlobalUnlock(this.data[index])
	medium.Data = unsafe.Pointer(hGlobal)
	return S_OK
}

func (this *DataObject) getDataHere(fetc *FORMATETC, medium *STGMEDIUM) HRESULT {
	return DV_E_FORMATETC
}

func (this *DataObject) queryGetData(fetc *FORMATETC) HRESULT {
	//core.Debug(fetc.cfFormat.String(), *fetc)
	if this.lookupFormatEtc(fetc) == -1 {
		return DV_E_FORMATETC
	}
	return S_OK
}

func (this *DataObject) getCanonicalFormatEtc(fetcIn *FORMATETC, fetcOut *FORMATETC) HRESULT {
	fetcOut.ptd = nil
	return E_NOTIMPL
}

func (this *DataObject) setData(fetc *FORMATETC, medium *STGMEDIUM, fRelease int32) HRESULT {
	return E_UNEXPECTED
}

func (this *DataObject) enumFormatEtc(dwDirection uint32, ppEnumFormatEtc **IEnumFORMATETC) HRESULT {
	if dwDirection != 1 /*DATADIR_GET*/ {
		return E_NOTIMPL
	}

	SHCreateStdEnumFmtEtc(uint32(len(this.data)), &this.formatEtcs[0], ppEnumFormatEtc)

	return S_OK
}

func (this *DataObject) dAdvise(fetc *FORMATETC, advf uint32, pAdvSink *IAdviseSink, pdwConnection *uint32) HRESULT {
	return E_FAIL
}

func (this *DataObject) dUnadvise(dwConnection uint32) HRESULT {
	return E_FAIL
}

func (this *DataObject) enumDAdvise(ppenumAdvise **IEnumSTATDATA) HRESULT {
	return E_FAIL
}

func destroyDataObject(this *DataObject) {
	for _, v := range this.data {
		GlobalFree(v)
	}
}

func (this *DataObject) From() interface{} {
	return this.from
}

var vtabIDataObject = IDataObjectVtbl{
	syscall.NewCallback((*DataObject).queryInterface),
	syscall.NewCallback((*DataObject).addRef),
	syscall.NewCallback((*DataObject).release),
	syscall.NewCallback((*DataObject).getData),
	syscall.NewCallback((*DataObject).getDataHere),
	syscall.NewCallback((*DataObject).queryGetData),
	syscall.NewCallback((*DataObject).getCanonicalFormatEtc),
	syscall.NewCallback((*DataObject).setData),
	syscall.NewCallback((*DataObject).enumFormatEtc),
	syscall.NewCallback((*DataObject).dAdvise),
	syscall.NewCallback((*DataObject).dUnadvise),
	syscall.NewCallback((*DataObject).enumDAdvise)}

func NewDataObject(cfs []CLIPFORMAT, data []HGLOBAL, from interface{}) *DataObject {
	if len(cfs) != len(data) {
		panic("len(cfs) != len(data) ")
	}
	if len(cfs) == 0 {
		return nil
	}
	p := new(DataObject)
	p.vtab = &vtabIDataObject
	p.addRef()
	p.data = data
	p.from = from
	for _, id := range cfs {
		p.formatEtcs = append(p.formatEtcs, FORMATETC{
			cfFormat: id,
			dwAspect: 1, /*DVASPECT_CONTENT*/
			lindex:   -1,
			tymed:    TYMED_HGLOBAL})
	}
	runtime.SetFinalizer(p, destroyDataObject)
	return p
}

/*	DVASPECT_CONTENT	= 1,
	DVASPECT_THUMBNAIL	= 2,
	DVASPECT_ICON	= 4,
	DVASPECT_DOCPRINT	= 8
    } 	DVASPECT;

*/

//            __RPC__in IDataObject * This,
//            /* [in] */ __RPC__in FORMATETC *pformatetc,
//            /* [in] */ DWORD advf,
//            /* [unique][in] */ __RPC__in_opt IAdviseSink *pAdvSink,
//            /* [out] */ __RPC__out DWORD *pdwConnection);
//            IDataObject * This,
//            /* [unique][in] */ FORMATETC *pformatetcIn,
//            /* [out] */ STGMEDIUM *pmedium);
//typedef struct IDataObjectVtbl
//    {
//        BEGIN_INTERFACE

//        HRESULT ( STDMETHODCALLTYPE *QueryInterface )(
//            __RPC__in IDataObject * This,
//            /* [in] */ __RPC__in REFIID riid,
//            /* [annotation][iid_is][out] */
//            __RPC__deref_out  void **ppvObject);

//        ULONG ( STDMETHODCALLTYPE *AddRef )(
//            __RPC__in IDataObject * This);

//        ULONG ( STDMETHODCALLTYPE *Release )(
//            __RPC__in IDataObject * This);

//        /* [local] */ HRESULT ( STDMETHODCALLTYPE *GetData )(
//            IDataObject * This,
//            /* [unique][in] */ FORMATETC *pformatetcIn,
//            /* [out] */ STGMEDIUM *pmedium);

//        /* [local] */ HRESULT ( STDMETHODCALLTYPE *GetDataHere )(
//            IDataObject * This,
//            /* [unique][in] */ FORMATETC *pformatetc,
//            /* [out][in] */ STGMEDIUM *pmedium);

//        HRESULT ( STDMETHODCALLTYPE *QueryGetData )(
//            __RPC__in IDataObject * This,
//            /* [unique][in] */ __RPC__in_opt FORMATETC *pformatetc);

//        HRESULT ( STDMETHODCALLTYPE *GetCanonicalFormatEtc )(
//            __RPC__in IDataObject * This,
//            /* [unique][in] */ __RPC__in_opt FORMATETC *pformatectIn,
//            /* [out] */ __RPC__out FORMATETC *pformatetcOut);

//        /* [local] */ HRESULT ( STDMETHODCALLTYPE *SetData )(
//            IDataObject * This,
//            /* [unique][in] */ FORMATETC *pformatetc,
//            /* [unique][in] */ STGMEDIUM *pmedium,
//            /* [in] */ BOOL fRelease);

//        HRESULT ( STDMETHODCALLTYPE *EnumFormatEtc )(
//            __RPC__in IDataObject * This,
//            /* [in] */ DWORD dwDirection,
//            /* [out] */ __RPC__deref_out_opt IEnumFORMATETC **ppenumFormatEtc);

//        HRESULT ( STDMETHODCALLTYPE *DAdvise )(
//            __RPC__in IDataObject * This,
//            /* [in] */ __RPC__in FORMATETC *pformatetc,
//            /* [in] */ DWORD advf,
//            /* [unique][in] */ __RPC__in_opt IAdviseSink *pAdvSink,
//            /* [out] */ __RPC__out DWORD *pdwConnection);

//        HRESULT ( STDMETHODCALLTYPE *DUnadvise )(
//            __RPC__in IDataObject * This,
//            /* [in] */ DWORD dwConnection);

//        HRESULT ( STDMETHODCALLTYPE *EnumDAdvise )(
//            __RPC__in IDataObject * This,
//            /* [out] */ __RPC__deref_out_opt IEnumSTATDATA **ppenumAdvise);

type IDropTargetVtbl struct {
	QueryInterface uintptr
	AddRef         uintptr
	Release        uintptr
	DragEnter      uintptr
	DragOver       uintptr
	DragLeave      uintptr
	Drop           uintptr
}

type IDropTarget interface {
	DragEnter(pDataObj *IDataObject,
		grfKeyState uint32, x int32, y int32, pdwEffect *uint32) HRESULT //E_UNEXPECTED, E_INVALIDARG, E_OUTOFMEMORY
	DragOver(grfKeyState uint32,
		x int32, y int32, pdwEffect *uint32) HRESULT //E_UNEXPECTED, E_INVALIDARG, E_OUTOFMEMORY
	DragLeave() HRESULT // E_OUTOFMEMORY
	Drop(pDataObj *IDataObject,
		grfKeyState uint32, x int32, y int32, pdwEffect *uint32) HRESULT //E_UNEXPECTED, E_INVALIDARG, E_OUTOFMEMORY
	HWND() HWND
}

type dropTarget struct {
	vtab *IDropTargetVtbl
	//refCount int32
	iface IDropTarget
}

func (this *dropTarget) queryInterface(riid *GUID, ppvObject *unsafe.Pointer) HRESULT {
	//return ComQueryInterface((*IUnknown)(unsafe.Pointer(this)), id)
	*ppvObject = nil
	if IsEqualGUID(riid, IID_IUnknown) {
		this.addRef()
		*ppvObject = unsafe.Pointer(this)
		return S_OK
	}
	if IsEqualGUID(riid, IID_IDropTarget) {
		this.addRef()
		*ppvObject = unsafe.Pointer(this)
		return S_OK
	}
	return E_NOINTERFACE
}

func (this *dropTarget) addRef() HRESULT {
	return HRESULT(lockInc(unsafe.Pointer(this)))
}

func (this *dropTarget) release() HRESULT {
	return HRESULT(lockDec(unsafe.Pointer(this)))
}

func (this *dropTarget) dragEnter(pDataObj *IDataObject,
	grfKeyState uint32, x int32, y int32, pdwEffect *uint32) HRESULT {
	return this.iface.DragEnter(pDataObj, grfKeyState, x, y, pdwEffect)
}
func (this *dropTarget) dragOver(grfKeyState uint32,
	x int32, y int32, pdwEffect *uint32) HRESULT {
	return this.iface.DragOver(grfKeyState, x, y, pdwEffect)
}
func (this *dropTarget) dragLeave() HRESULT {
	return this.iface.DragLeave()
}
func (this *dropTarget) drop(pDataObj *IDataObject,
	grfKeyState uint32, x int32, y int32, pdwEffect *uint32) HRESULT {
	return this.iface.Drop(pDataObj, grfKeyState, x, y, pdwEffect)
}

var vtabIDropTarget = IDropTargetVtbl{
	syscall.NewCallback((*dropTarget).queryInterface),
	syscall.NewCallback((*dropTarget).addRef),
	syscall.NewCallback((*dropTarget).release),
	syscall.NewCallback((*dropTarget).dragEnter),
	syscall.NewCallback((*dropTarget).dragOver),
	syscall.NewCallback((*dropTarget).dragLeave),
	syscall.NewCallback((*dropTarget).drop)}

func RegisterDragDrop(target IDropTarget) error {
	OleInitialize()

	p := new(dropTarget)
	s := (*dropTarget)(p)
	s.vtab = &vtabIDropTarget
	//	s.refCount = 0
	s.iface = target

	ret, _, _ := procRegisterDragDrop.Call(
		uintptr(target.HWND()),
		uintptr(unsafe.Pointer(s)))
	if ret == S_OK {
		return nil
	}
	return errors.New(HRESULT(ret).String())
}

func RevokeDragDrop(target IDropTarget) error {
	ret, _, _ := procRevokeDragDrop.Call(
		uintptr(target.HWND()))
	if ret == S_OK {
		return nil
	}
	return errors.New(HRESULT(ret).String())
}

//typedef struct IDropTargetVtbl
//{
//    BEGIN_INTERFACE

//    HRESULT ( STDMETHODCALLTYPE *QueryInterface )(
//        __RPC__in IDropTarget * This,
//        /* [in] */ __RPC__in REFIID riid,
//        /* [annotation][iid_is][out] */
//        __RPC__deref_out  void **ppvObject);

//    ULONG ( STDMETHODCALLTYPE *AddRef )(
//        __RPC__in IDropTarget * This);

//    ULONG ( STDMETHODCALLTYPE *Release )(
//        __RPC__in IDropTarget * This);

//    HRESULT ( STDMETHODCALLTYPE *DragEnter )(
//        __RPC__in IDropTarget * This,
//        /* [unique][in] */ __RPC__in_opt IDataObject *pDataObj,
//        /* [in] */ DWORD grfKeyState,
//        /* [in] */ POINTL pt,
//        /* [out][in] */ __RPC__inout DWORD *pdwEffect);

//    HRESULT ( STDMETHODCALLTYPE *DragOver )(
//        __RPC__in IDropTarget * This,
//        /* [in] */ DWORD grfKeyState,
//        /* [in] */ POINTL pt,
//        /* [out][in] */ __RPC__inout DWORD *pdwEffect);

//    HRESULT ( STDMETHODCALLTYPE *DragLeave )(
//        __RPC__in IDropTarget * This);

//    HRESULT ( STDMETHODCALLTYPE *Drop )(
//        __RPC__in IDropTarget * This,
//        /* [unique][in] */ __RPC__in_opt IDataObject *pDataObj,
//        /* [in] */ DWORD grfKeyState,
//        /* [in] */ POINTL pt,
//        /* [out][in] */ __RPC__inout DWORD *pdwEffect);

//    END_INTERFACE
//} IDropTargetVtbl;

//interface IDropTarget
//{
//    CONST_VTBL struct IDropTargetVtbl *lpVtbl;
//};

//type IDropSourceNotifyVtbl struct {
//	QueryInterface  uintptr
//	AddRef          uintptr
//	Release         uintptr
//	DragEnterTarget uintptr
//	DragLeaveTarget uintptr
//}

//type IDropSourceNotify struct {
//	lpVtbl *IDropSourceNotifyVtbl
//
//}

//typedef struct IDropSourceNotifyVtbl
//{
//    BEGIN_INTERFACE

//    HRESULT ( STDMETHODCALLTYPE *QueryInterface )(
//        IDropSourceNotify * This,
//        /* [in] */ REFIID riid,
//        /* [annotation][iid_is][out] */
//        __RPC__deref_out  void **ppvObject);

//    ULONG ( STDMETHODCALLTYPE *AddRef )(
//        IDropSourceNotify * This);

//    ULONG ( STDMETHODCALLTYPE *Release )(
//        IDropSourceNotify * This);

//    HRESULT ( STDMETHODCALLTYPE *DragEnterTarget )(
//        IDropSourceNotify * This,
//        /* [annotation][in] */
//        __in  HWND hwndTarget);

//    HRESULT ( STDMETHODCALLTYPE *DragLeaveTarget )(
//        IDropSourceNotify * This);

//    END_INTERFACE
//} IDropSourceNotifyVtbl;

//interface IDropSourceNotify
//{
//    CONST_VTBL struct IDropSourceNotifyVtbl *lpVtbl;
//};

type IDropSourceVtbl struct {
	QueryInterface    uintptr
	AddRef            uintptr
	Release           uintptr
	QueryContinueDrag uintptr
	GiveFeedback      uintptr
}

type DropSource struct {
	vtab    *IDropSourceVtbl
	feeback func(uint32)
}

func (this *DropSource) queryInterface(riid *GUID, ppvObject *unsafe.Pointer) HRESULT {
	//return ComQueryInterface((*IUnknown)(unsafe.Pointer(this)), id)
	*ppvObject = nil
	if IsEqualGUID(riid, IID_IUnknown) {
		this.addRef()
		*ppvObject = unsafe.Pointer(this)
		return S_OK
	}
	if IsEqualGUID(riid, IID_IDropTarget) {
		this.addRef()
		*ppvObject = unsafe.Pointer(this)
		return S_OK
	}
	return E_NOINTERFACE
}

func (this *DropSource) addRef() HRESULT {
	return HRESULT(lockInc(unsafe.Pointer(this)))
}

func (this *DropSource) release() HRESULT {
	return HRESULT(lockDec(unsafe.Pointer(this)))
}

func (this *DropSource) queryContinueDrag(fEscapePressed int32, grfKeyState uint32) HRESULT {
	if grfKeyState&MK_LBUTTON == 0 {
		return DRAGDROP_S_DROP
	}
	return S_OK
}

func (this *DropSource) giveFeedback(dwEffect uint32) HRESULT {
	if this.feeback != nil {
		this.feeback(dwEffect & 0xff)
		return S_OK
	}
	return DRAGDROP_S_USEDEFAULTCURSORS
}

var vtabIDropSource = IDropSourceVtbl{
	syscall.NewCallback((*DropSource).queryInterface),
	syscall.NewCallback((*DropSource).addRef),
	syscall.NewCallback((*DropSource).release),
	syscall.NewCallback((*DropSource).queryContinueDrag),
	syscall.NewCallback((*DropSource).giveFeedback)}

func NewDropSource(feeback func(uint32)) *DropSource {
	p := new(DropSource)
	p.vtab = &vtabIDropSource
	p.feeback = feeback
	return p
}

//typedef struct IDropSourceVtbl
// {
//     BEGIN_INTERFACE

//     HRESULT ( STDMETHODCALLTYPE *QueryInterface )(
//         IDropSource * This,
//         /* [in] */ REFIID riid,
//         /* [annotation][iid_is][out] */
//         __RPC__deref_out  void **ppvObject);

//     ULONG ( STDMETHODCALLTYPE *AddRef )(
//         IDropSource * This);

//     ULONG ( STDMETHODCALLTYPE *Release )(
//         IDropSource * This);

//     HRESULT ( STDMETHODCALLTYPE *QueryContinueDrag )(
//         IDropSource * This,
//         /* [annotation][in] */
//         __in  BOOL fEscapePressed,
//         /* [annotation][in] */
//         __in  DWORD grfKeyState);

//     HRESULT ( STDMETHODCALLTYPE *GiveFeedback )(
//         IDropSource * This,
//         /* [annotation][in] */
//         __in  DWORD dwEffect);

//     END_INTERFACE
// } IDropSourceVtbl;

// interface IDropSource
// {
//     CONST_VTBL struct IDropSourceVtbl *lpVtbl;
// };

//func (this *IEnumFORMATETC) AddRef() int {
//	p := C.IUnknown_addref(unsafe.Pointer(this), unsafe.Pointer(this.vtab.AddRef))
//	return int(uintptr(p))
//}

//func (this *IEnumFORMATETC) Release() int {
//	// same as IUnknown_addref, use it
//	p := C.IUnknown_addref(unsafe.Pointer(this), unsafe.Pointer(this.vtab.Release))
//	return int(uintptr(p))
//}
/*
func (this *IEnumFORMATETC) Next(celt uint32, rgelt *FORMATETC, pceltFetched *uint32) HRESULT {

}

func (this *IEnumFORMATETC) Skip(celt uint32) HRESULT {

}

func (this *IEnumFORMATETC) Reset() HRESULT {

}

func (this *IEnumFORMATETC) Clone(penum *IEnumFORMATETC) HRESULT {

}
*/

//typedef struct IEnumFORMATETCVtbl
//    {
//        BEGIN_INTERFACE

//        HRESULT ( STDMETHODCALLTYPE *QueryInterface )(
//            __RPC__in IEnumFORMATETC * This,
//            /* [in] */ __RPC__in REFIID riid,
//            /* [annotation][iid_is][out] */
//            __RPC__deref_out  void **ppvObject);

//        ULONG ( STDMETHODCALLTYPE *AddRef )(
//            __RPC__in IEnumFORMATETC * This);

//        ULONG ( STDMETHODCALLTYPE *Release )(
//            __RPC__in IEnumFORMATETC * This);

//        /* [local] */ HRESULT ( STDMETHODCALLTYPE *Next )(
//            IEnumFORMATETC * This,
//            /* [in] */ ULONG celt,
//            /* [annotation] */
//            __out_ecount_part(celt,*pceltFetched)  FORMATETC *rgelt,
//            /* [annotation] */
//            __out_opt  ULONG *pceltFetched);

//        HRESULT ( STDMETHODCALLTYPE *Skip )(
//            __RPC__in IEnumFORMATETC * This,
//            /* [in] */ ULONG celt);

//        HRESULT ( STDMETHODCALLTYPE *Reset )(
//            __RPC__in IEnumFORMATETC * This);

//        HRESULT ( STDMETHODCALLTYPE *Clone )(
//            __RPC__in IEnumFORMATETC * This,
//            /* [out] */ __RPC__deref_out_opt IEnumFORMATETC **ppenum);

//        END_INTERFACE
//    } IEnumFORMATETCVtbl;
