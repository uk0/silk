package hmi_test

import (
	"context"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/hmi"
	pointsql "github.com/uk0/silk/point/sqlite"
	"path/filepath"
	"testing"
	"time"
)

func TestRuntimePersistentDatabaseBindingAndRetention(t *testing.T) {
	if !pointsql.StoreAvailable() {
		t.Skip("SQLite requires CGO")
	}
	ctx := context.Background()
	store, err := pointsql.Open(filepath.Join(t.TempDir(), "points.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p := hmi.Template()
	rt, err := hmi.NewRuntime(ctx, p, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	old := core.Value{Raw: "91.25", Quality: core.QualityGood, Time: now.Add(-48 * time.Hour)}
	if err = rt.Publish(ctx, "AI101", old); err != nil {
		t.Fatal(err)
	}
	fresh := core.Value{Raw: 93.5, Quality: core.QualityGood, Time: now}
	if err = rt.Publish(ctx, "AI101", fresh); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Latest(ctx, "AI101")
	if err != nil || latest.Value.Raw != 93.5 {
		t.Fatal(latest, err)
	}
	rt.Stop()
	if rt.Publish(ctx, "AI101", fresh) == nil {
		t.Fatal("closed runtime accepted publish")
	}
	rt, err = hmi.NewRuntime(ctx, p, store, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Stop()
	tag, _ := rt.Tags.Get("AI101")
	if tag.Value().Raw != 93.5 || !tag.Value().Time.Equal(now) {
		t.Fatal("database binding failed to restore", tag.Value())
	}
	n, err := store.Prune(ctx, now.Add(-24*time.Hour), 1)
	if err != nil || n != 1 {
		t.Fatal("bounded retention", n, err)
	}
	history, err := store.History(ctx, "AI101", now.Add(-72*time.Hour), now.Add(time.Second), 10)
	if err != nil || len(history) != 1 || history[0].Value.Raw != 93.5 {
		t.Fatal(history, err)
	}
	latest, err = store.Latest(ctx, "AI101")
	if err != nil || latest.Value.Raw != 93.5 {
		t.Fatal("prune removed latest", err)
	}
}
