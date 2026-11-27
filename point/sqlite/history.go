package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/uk0/silk/point"
)

// ReadHistory adapts the existing History API without changing StoredSample's
// identity or the old range/order contract.
func (s *Store) ReadHistory(ctx context.Context, name string, from, to time.Time, limit int) ([]point.SampleRecord, error) {
	if ctx == nil {
		return nil, fmt.Errorf("point: nil history context")
	}
	rows, err := s.History(ctx, name, from, to, limit)
	if err != nil {
		return nil, err
	}
	return sampleRecords(rows), nil
}

func sampleRecords(rows []StoredSample) []point.SampleRecord {
	out := make([]point.SampleRecord, len(rows))
	for i, row := range rows {
		out[i] = point.SampleRecord{Name: row.Name, Value: row.Value}
	}
	return out
}

// RecentHistory reads the newest bounded observations, not the first N rows of
// an arbitrarily chosen time window. No retention or write is performed here.
func (s *Store) RecentHistory(ctx context.Context, name string, limit int) ([]point.SampleRecord, error) {
	if ctx == nil || limit < 1 || limit > 10000 {
		return nil, fmt.Errorf("point: invalid recent history context/limit")
	}
	rows, err := s.readHistoryRows(ctx, name, `SELECT h.payload,h.quality,h.ts,d.kind FROM point_history h JOIN point_definitions d ON d.name=h.name WHERE h.name=? ORDER BY h.ts DESC,h.seq DESC LIMIT ?`, name, limit)
	if err != nil {
		return nil, err
	}
	return sampleRecords(rows), nil
}

// Enforce the encoded budget in both query directions before decoding a
// row. Returning no partial data avoids drawing a truncated historical trend
// that looks authoritative to the operator.
func (s *Store) readHistoryRows(ctx context.Context, name, query string, args ...any) ([]StoredSample, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredSample{}
	remaining := point.MaxHistoryPayloadBytes
	for rows.Next() {
		var payload string
		var quality int
		var ts int64
		var kind point.Kind
		if err := rows.Scan(&payload, &quality, &ts, &kind); err != nil {
			return nil, err
		}
		if len(payload) > remaining {
			return nil, point.ErrHistoryPayloadLimit
		}
		remaining -= len(payload)
		row, err := decodeSample(name, payload, quality, ts, kind)
		if err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

var _ point.HistoryReader = (*Store)(nil)
