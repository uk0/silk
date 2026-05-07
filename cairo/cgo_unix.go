//go:build !windows && !silk_no_cairo

// Unix-like systems (macOS, Linux, *BSD) discover Cairo headers and libs
// via pkg-config, which is the standard mechanism on these platforms.

package cairo

// #cgo pkg-config: cairo
import "C"
