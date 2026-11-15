package point

import (
	"context"
	"time"

	"github.com/uk0/silk/core"
)

// SampleRecord is a typed observation independent of a storage implementation.
type SampleRecord struct {
	Name  string
	Value core.Value
}

// HistoryReader is an optional runtime store capability. Range results are
// oldest-first; recent results are newest-first, with insertion order breaking
// equal-time ties. Implementations must respect cancellation and bounded limits.
type HistoryReader interface {
	ReadHistory(context.Context, string, time.Time, time.Time, int) ([]SampleRecord, error)
	RecentHistory(context.Context, string, int) ([]SampleRecord, error)
}
