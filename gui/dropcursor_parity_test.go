package gui

import (
	"github.com/uk0/silk/paint"
	"reflect"
	"testing"
)

func TestDropCursorsSurviveMissingResources(t *testing.T) {
	old := dndCursorData
	dndCursorData = make([]CursorData, 4)
	defer func() { dndCursorData = old }()
	curs := generateDropCursors(nil, func(CursorData) (*Cursor, error) { t.Fatal("invalid pixels reached native creation"); return nil, nil })
	for _, cur := range curs {
		if cur == nil {
			t.Fatal("missing pixels produced a nil cursor")
		}
	}
	ReleaseDropCursors(curs)
}

func TestDropCursorTemporaryPixelsRetiredBeforeReturn(t *testing.T) {
	var kept []paint.Pixmap
	generateDropCursors(nil, func(data CursorData) (*Cursor, error) { kept = append(kept, data.Pixmap); return nil, nil })
	if len(kept) != 4 {
		t.Fatal("missing native creations")
	}
	for _, pix := range kept {
		if !backbufferSurfaceRetiredForCursorTest(pix) {
			t.Fatal("temporary surface waits for GC")
		}
	}
}

func backbufferSurfaceRetiredForCursorTest(p paint.Pixmap) bool {
	v := reflect.ValueOf(p).Elem().FieldByName("Surface")
	return v.IsValid() && v.Kind() == reflect.Pointer && v.IsNil()
}
