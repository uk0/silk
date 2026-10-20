package win32

import (
	"fmt"
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// Catch copy/paste misrouting even when the public signature still compiles.
// Aliases such as LoadIconResource have no matching proc variable and are not
// required to call a same-named entry point.
func TestWin32ForwardersUseMatchingProcedures(t *testing.T) {
	checked := 0
	for _, path := range []string{"user32.go", "gdi32.go", "kernel32.go", "imm32_windows.go"} {
		file, err := parser.ParseFile(gotoken.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		procs := make(map[string]bool)
		for _, decl := range file.Decls {
			if group, ok := decl.(*ast.GenDecl); ok && group.Tok == gotoken.VAR {
				for _, spec := range group.Specs {
					for _, name := range spec.(*ast.ValueSpec).Names {
						procs[name.Name] = true
					}
				}
			}
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || !procs["proc"+fn.Name.Name] {
				continue
			}
			calls, correct := 0, false
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				selector, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "Call" {
					return true
				}
				ident, ok := selector.X.(*ast.Ident)
				if ok && procs[ident.Name] {
					calls++
					correct = correct || ident.Name == "proc"+fn.Name.Name
				}
				return true
			})
			if calls != 0 {
				checked++
				if !correct {
					t.Errorf("%s: %s does not call proc%s", path, fn.Name.Name, fn.Name.Name)
				}
			}
		}
	}
	if checked < 100 {
		t.Fatalf("forwarder guard did not cover enough native wrappers: %d", checked)
	}
	t.Logf("checked %d same-name native forwarders", checked)
}

func TestStockObjectMatchesNativeGDI(t *testing.T) {
	for _, object := range []int{BLACK_BRUSH, NULL_BRUSH, DEFAULT_GUI_FONT} {
		want, _, _ := procGetStockObject.Call(uintptr(object))
		if got := GetStockObject(object); want == 0 || uintptr(got) != want {
			t.Errorf("stock object %d = %v, native GetStockObject=%v", object, got, want)
		}
	}
}

func TestMapVirtualKeyExMatchesExplicitLayout(t *testing.T) {
	layout, _, _ := moduser32.NewProc("GetKeyboardLayout").Call(0)
	if layout == 0 {
		t.Fatal("native keyboard layout unavailable")
	}
	for _, key := range []uint{'A', 'Z', '1', VK_RETURN} {
		for _, mapping := range []uint{0, 1, 2, 3, 4} {
			want, _, _ := procMapVirtualKeyEx.Call(uintptr(key), uintptr(mapping), layout)
			if got := MapVirtualKeyEx(key, mapping, HKL(layout)); uintptr(got) != want {
				t.Errorf("key=%d mapping=%d layout=%x: got=%v native=%v", key, mapping, layout, got, want)
			}
		}
	}
}

func TestTranslateAcceleratorDispatchesNativeCommand(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance := GetModuleHandle("")
	name := syscall.StringToUTF16Ptr(fmt.Sprintf("SilkAcceleratorTest%d", time.Now().UnixNano()))
	commands := 0
	callback := syscall.NewCallback(func(h HWND, message uint32, w, l uintptr) uintptr {
		if message == WM_COMMAND && w&0xffff == 0x7345 && w>>16 == 1 && l == 0 {
			commands++
			return 0
		}
		return DefWindowProc(h, message, w, l)
	})
	class := WNDCLASSEX{Size: uint32(unsafe.Sizeof(WNDCLASSEX{})), Instance: instance, ClassName: name, WndProc: callback}
	if RegisterClassEx(&class) == 0 {
		t.Fatal("RegisterClassEx failed")
	}
	defer moduser32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(name)), uintptr(instance))
	h := CreateWindowEx(0, name, nil, WS_POPUP, 0, 0, 64, 48, 0, 0, instance, nil)
	if h == 0 {
		t.Fatal("CreateWindowEx failed")
	}
	defer DestroyWindow(h)
	// ACCEL records have a six-byte layout, but Win10 probes the table's
	// base at DWORD alignment. A Go struct aligned only to WORD can land at
	// address % 4 == 2 under -race and fail with ERROR_NOACCESS. Use aligned
	// storage for this single record without inventing an eight-byte array stride.
	accel := [2]uint32{uint32(VK_F12)<<16 | 1, 0x7345}
	table, _, tableErr := moduser32.NewProc("CreateAcceleratorTableW").Call(uintptr(unsafe.Pointer(&accel[0])), 1)
	runtime.KeepAlive(accel)
	if table == 0 {
		t.Fatalf("CreateAcceleratorTableW failed: %v", tableErr)
	}
	defer moduser32.NewProc("DestroyAcceleratorTable").Call(table)
	msg := MSG{Hwnd: h, Message: WM_KEYDOWN, WParam: VK_F12}
	if !TranslateAccelerator(h, HACCEL(table), &msg) || commands != 1 {
		t.Fatalf("accelerator did not dispatch exactly one native command: %d", commands)
	}
	msg.WParam = VK_F11
	if TranslateAccelerator(h, HACCEL(table), &msg) || commands != 1 {
		t.Fatal("unbound key was consumed or dispatched")
	}
}
