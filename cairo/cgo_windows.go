//go:build !silk_pure_go

// Windows cgo flags for Cairo.
//
// Resolved through pkg-config, exactly like the unix build (cgo_unix.go).
//
// This file used to hardcode the MSYS2 MinGW64 prefix:
//
//	#cgo CFLAGS: -IC:/msys64/mingw64/include -IC:/msys64/mingw64/include/cairo
//
// which broke in two ways. Silk moved to the UCRT64 toolchain, so
// .../mingw64/... no longer exists on a machine that installed only the
// documented mingw-w64-ucrt-x86_64-* packages; and MSYS2 is not always at
// C:\msys64 — GitHub's setup-msys2 action may install it under RUNNER_TEMP.
// Either way cgo failed with "cairo.h: No such file or directory".
//
// pkg-config reports the right -I (including the .../include/cairo directory
// that makes the plain `#include <cairo.h>` resolve) and the right -L for
// whatever prefix and toolchain is actually installed. Windows setup already
// requires it — README's Windows section installs mingw-w64-ucrt-x86_64-pkgconf
// and verifies with `pkg-config --modversion cairo`.
//
// To point the build at a Cairo that pkg-config does not know about, set the
// usual environment variables; cgo appends them to the flags below:
//
//	set CGO_CFLAGS=-IC:/your/cairo/include/cairo
//	set CGO_LDFLAGS=-LC:/your/cairo/lib -lcairo

package cairo

// #cgo pkg-config: cairo
import "C"
