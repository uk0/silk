package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
	"github.com/uk0/silk/internal/fileops"
	"io"
	"os"
)

// File syscalls run only on a bounded worker. A canceled OS open may remain
// blocked; no reader is replaced until it really returns. Ordinary manual opens
// retain their old API and are not claimed to be nonblocking by this increment.
func readRestoreBytes(ctx context.Context, path string) ([]byte, error) {
	data, _, err := readRestoreSnapshot(ctx, path)
	return data, err
}
func readRestoreSnapshot(ctx context.Context, path string) ([]byte, os.FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	before, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%w: %s", errRestoreNotRegular, path)
	}
	if before.Size() > restoreFileLimit {
		return nil, nil, fmt.Errorf("restore: file exceeds 32 MiB limit: %s", path)
	}
	f, err := fileops.OpenSnapshot(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, nil, fmt.Errorf("restore: file identity changed: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(restoreContextReader{ctx, f}, restoreFileLimit+1))
	if err != nil {
		return nil, nil, err
	}
	if len(data) > restoreFileLimit {
		return nil, nil, fmt.Errorf("restore: file grew past 32 MiB limit: %s", path)
	}
	after, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if opened.Size() != after.Size() || opened.ModTime() != after.ModTime() {
		return nil, nil, fmt.Errorf("restore: file changed while reading: %s", path)
	}
	return data, after, ctx.Err()
}

type restoreContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r restoreContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
func decodeRestoreDesign(ctx context.Context, data []byte) (*core.TDoc, error) {
	if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
		z, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, err
		}
		z.Multistream(false)
		data, err = io.ReadAll(io.LimitReader(restoreContextReader{ctx, z}, restoreFileLimit+1))
		err = errors.Join(err, z.Close())
		if err != nil {
			return nil, err
		}
		if len(data) > restoreFileLimit {
			return nil, fmt.Errorf("restore: expanded design exceeds 32 MiB")
		}
		if len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b {
			return nil, fmt.Errorf("restore: nested gzip design")
		}
	}
	doc, err := core.LoadTDoc1(restoreContextReader{ctx, bytes.NewReader(data)}, restoreNodeLimit+1)
	if err != nil {
		return nil, err
	}
	nodes := []*core.TDoc{doc}
	count := 0
	for len(nodes) > 0 {
		n := nodes[len(nodes)-1]
		nodes = nodes[:len(nodes)-1]
		count++
		if count > restoreNodeLimit {
			return nil, fmt.Errorf("restore: design exceeds node limit")
		}
		for i := 0; i < n.Len(); i++ {
			nodes = append(nodes, n.Child(i))
		}
	}
	return doc, ctx.Err()
}
func readRestoredFile(ctx context.Context, path string, design bool) (restoredFile, error) {
	data, info, err := readRestoreSnapshot(ctx, path)
	if err != nil {
		return restoredFile{}, err
	}
	if !design {
		return restoredFile{data: data}, nil
	}
	doc, err := decodeRestoreDesign(ctx, data)
	if err != nil {
		return restoredFile{}, err
	}
	value := restoredFile{doc: doc, modified: info.ModTime(), size: info.Size()}
	// Recovery metadata, reading and decoding stay off the UI thread too. The UI
	// later asks the existing explicit recovery question using these snapshots.
	if recovery := ged.CheckRecovery(path); recovery != "" {
		value.recoveryPath = recovery
		var b []byte
		b, value.recoveryErr = readRestoreBytes(ctx, recovery)
		if value.recoveryErr == nil {
			value.recovery, value.recoveryErr = decodeRestoreDesign(ctx, b)
		}
	}
	return value, ctx.Err()
}
