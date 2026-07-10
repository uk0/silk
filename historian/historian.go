// Package historian turns silk's live-only tag trends into reviewable history:
// it records core.Tag changes into a SQLite table and answers time-range
// queries, so a screen that only shows the current value can be backed by a
// scrollable, persisted trend.
package historian

import (
	"database/sql"
	"time"

	"github.com/uk0/silk/core"
	_ "github.com/mattn/go-sqlite3" // registers the "sqlite3" driver (repo idiom)
)

// Historian owns a SQLite handle holding a single samples table.
type Historian struct {
	db *sql.DB
}

// NewHistorian opens (creating if needed) the SQLite database at dbPath and
// makes sure the samples table and its (tag, ts) index exist.
func NewHistorian(dbPath string) (*Historian, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	// SQLite is a single-writer store; cap the pool at one connection so inserts
	// arriving from any goroutine and range queries serialise through the
	// database/sql pool instead of racing (no "database is locked").
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS samples (tag TEXT NOT NULL, ts INTEGER NOT NULL, value REAL NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS samples_tag_ts ON samples (tag, ts)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Historian{db: db}, nil
}

// Record subscribes to each named tag and writes one row per change; the
// returned func unsubscribes them all. Subscribe primes each callback with the
// tag's current value, so recording opens with a sample of the tag's state at
// record-start, followed by one per subsequent change.
func (h *Historian) Record(tags *core.TagDB, names []string) func() {
	cancels := make([]core.CancelFunc, 0, len(names))
	for _, name := range names {
		tag := tags.GetOrCreate(name, core.Meta{})
		cancels = append(cancels, tag.Subscribe(func(v core.Value) {
			// Best-effort: the tag callback fires on arbitrary goroutines and has
			// no error channel, so a failed insert (e.g. after Close) is dropped.
			_, _ = h.db.Exec(
				`INSERT INTO samples (tag, ts, value) VALUES (?, ?, ?)`,
				name, time.Now().UnixNano(), v.Float())
		}))
	}
	return func() {
		for _, cancel := range cancels {
			cancel()
		}
	}
}

// Sample is one recorded point: its wall-clock time and float value.
type Sample struct {
	TS    time.Time
	Value float64
}

// Query returns tag's samples whose timestamp is in [from, to] inclusive,
// ordered by timestamp ascending.
func (h *Historian) Query(tag string, from, to time.Time) ([]Sample, error) {
	rows, err := h.db.Query(
		`SELECT ts, value FROM samples WHERE tag = ? AND ts BETWEEN ? AND ? ORDER BY ts ASC`,
		tag, from.UnixNano(), to.UnixNano())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Sample
	for rows.Next() {
		var (
			ts    int64
			value float64
		)
		if err := rows.Scan(&ts, &value); err != nil {
			return nil, err
		}
		out = append(out, Sample{TS: time.Unix(0, ts), Value: value})
	}
	return out, rows.Err()
}

// Close releases the underlying database handle.
func (h *Historian) Close() error {
	return h.db.Close()
}
