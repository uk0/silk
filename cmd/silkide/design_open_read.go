package main

import (
	"context"
	"errors"

	"github.com/uk0/silk/core"
	"github.com/uk0/silk/ged"
)

type designOpenSnapshot struct {
	restoredFile
	sourceErr error
}

// Reloads never inspect recovery siblings or repeat a read during UI apply.
// A corrupt/missing replacement keeps the current scene and its history intact.
func readReloadDesign(ctx context.Context, path string) (designOpenSnapshot, error) {
	var value designOpenSnapshot
	data, info, err := readRestoreSnapshot(ctx, path)
	if err != nil {
		return value, err
	}
	value.modified, value.size = info.ModTime(), info.Size()
	value.doc, err = decodeManualDesign(ctx, data)
	return value, err
}

// Every stat/open/read/decompression/parse stays on the bounded worker. Primary
// and recovery failures are separate: a valid autosave can recover a missing or
// damaged original, but rejecting that recovery must not install an empty form.
func readManualDesign(ctx context.Context, path string) (designOpenSnapshot, error) {
	var value designOpenSnapshot
	data, info, err := readRestoreSnapshot(ctx, path)
	value.sourceErr = err
	if info != nil {
		value.modified, value.size = info.ModTime(), info.Size()
	}
	if err == nil {
		value.doc, value.sourceErr = decodeManualDesign(ctx, data)
	}
	if err := ctx.Err(); err != nil {
		return designOpenSnapshot{}, err
	}
	if recovery := ged.CheckRecovery(path); recovery != "" {
		value.recoveryPath = recovery
		data, value.recoveryErr = readRestoreBytes(ctx, recovery)
		if value.recoveryErr == nil {
			value.recovery, value.recoveryErr = decodeManualDesign(ctx, data)
		}
	}
	if err := ctx.Err(); err != nil {
		return designOpenSnapshot{}, err
	}
	if value.sourceErr != nil && value.recoveryPath == "" {
		return designOpenSnapshot{}, value.sourceErr
	}
	return value, nil
}

func decodeManualDesign(ctx context.Context, data []byte) (*core.TDoc, error) {
	doc, err := decodeRestoreDesign(ctx, data)
	if err != nil {
		return nil, err
	}
	var kind string
	if doc == nil || doc.Value(&kind) != nil || kind != "form" {
		return nil, errors.New("open: expected a silkui form document")
	}
	return doc, nil
}
