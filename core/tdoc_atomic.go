package core

import (
	"io"

	"github.com/uk0/silk/internal/fileops"
)

// SaveFileAtomic saves an independent TDoc without truncating an existing
// destination on encoding or I/O failure. See SaveFileAtomic1 for limitations.
// SaveFile retains its legacy direct-write semantics for compatibility.
func (this *TDoc) SaveFileAtomic(filename string) error {
	return this.SaveFileAtomic1(filename, false)
}

// SaveFileAtomic1 stages the whole document (including gzip trailer when gz is
// true), syncs/closes it, and replaces a regular destination in the same
// directory. It preserves existing permissions, rejects read-only files/leaf
// symlinks and detects ordinary changes during encoding. It does not lock out
// another process between the final check and replacement, copy extended ACLs,
// or guarantee every filesystem/power-loss case. New files use 0644.
//
// This is synchronous. A worker may call it only with an independent document
// that no other goroutine mutates until the call returns; never traverse live
// UI widgets on that worker to obtain the document.
func (this *TDoc) SaveFileAtomic1(filename string, gz bool) error {
	return fileops.WriteAtomic(filename, 0644, func(w io.Writer) error {
		return this.Save1(w, gz)
	})
}
