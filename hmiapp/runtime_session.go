package hmiapp

import (
	"context"
	"net/http"

	"github.com/uk0/silk/hmi"
	pointsql "github.com/uk0/silk/point/sqlite"
)

// runStoredRuntime belongs to a worker. The owning UI must revoke bindings and
// cancel ctx before joining; no worker waits for a UI callback to finish.
func runStoredRuntime(ctx context.Context, project hmi.Project, database string, headers func(string) http.Header, post func(context.Context, func()) error, ready func(*hmi.Runtime)) (result error) {
	if err := ctx.Err(); err != nil {
		return nil
	}
	store, err := pointsql.Open(database)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(); result == nil {
			result = err
		}
	}()
	runtime, err := hmi.NewRuntime(ctx, project, store, headers)
	if err != nil {
		return err
	}
	defer runtime.Stop()
	if err = runtime.Start(ctx); err != nil {
		return err
	}
	if err = post(ctx, func() {
		if ctx.Err() == nil {
			ready(runtime)
		}
	}); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	<-ctx.Done()
	return nil
}
