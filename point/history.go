package point

import (
	"context"
	"errors"
	"time"

	"github.com/uk0/silk/core"
)

// MaxHistoryPayloadBytes bounds the payload returned by one history read.
// The SQLite adapter enforces it on stored JSON bytes; the HMI runtime also
// enforces it on decoded strings from custom history readers.
const MaxHistoryPayloadBytes = 16 << 20

// ErrHistoryPayloadLimit asks the caller to use a smaller range or row limit.
var ErrHistoryPayloadLimit = errors.New("point: history exceeds 16 MiB payload budget")

// SampleRecord is a typed observation independent of a storage implementation.
type SampleRecord struct {
	Name  string
	Value core.Value
}

// HistoryReader is an optional runtime store capability. Range results are
// oldest-first; recent results are newest-first, with insertion order breaking
// equal-time ties. Implementations must respect cancellation, bounded row counts
// and the payload budget. A limit error must not return partial success.
type HistoryReader interface {
	ReadHistory(context.Context, string, time.Time, time.Time, int) ([]SampleRecord, error)
	RecentHistory(context.Context, string, int) ([]SampleRecord, error)
}
