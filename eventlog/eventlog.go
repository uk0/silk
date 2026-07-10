// Package eventlog records alarms, user actions and system events into a
// SQLite table and answers time-range queries, backing FameView's 事件记录
// (event/audit log) screen with a persisted, filterable history.
package eventlog

import (
	"database/sql"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3" // registers the "sqlite3" driver (repo idiom)
)

// Kind classifies an event by what produced it.
type Kind string

const (
	KindAlarm  Kind = "alarm"  // a threshold/condition alarm fired
	KindLogin  Kind = "login"  // a user authentication event
	KindWrite  Kind = "write"  // an operator/system write to a tag or value
	KindSystem Kind = "system" // a system lifecycle or internal event
)

// Event is one recorded log entry: when it happened, its kind, the component
// that emitted it, and a human-readable message.
type Event struct {
	TS      time.Time
	Kind    Kind
	Source  string
	Message string
}

// Log owns a SQLite handle holding a single events table.
type Log struct {
	db *sql.DB
}

// NewLog opens (creating if needed) the SQLite database at dbPath and makes
// sure the events table and its ts and kind indexes exist.
func NewLog(dbPath string) (*Log, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}
	// SQLite is a single-writer store; cap the pool at one connection so records
	// arriving from any goroutine and range queries serialise through the
	// database/sql pool instead of racing (no "database is locked").
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS events (ts INTEGER, kind TEXT, source TEXT, message TEXT)`,
		`CREATE INDEX IF NOT EXISTS events_ts ON events (ts)`,
		`CREATE INDEX IF NOT EXISTS events_kind ON events (kind)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Log{db: db}, nil
}

// Record appends one event stamped with the current wall-clock time. It is safe
// to call from any goroutine: writes serialise through the single-connection
// database/sql pool.
func (l *Log) Record(kind Kind, source, message string) error {
	_, err := l.db.Exec(
		`INSERT INTO events (ts, kind, source, message) VALUES (?, ?, ?, ?)`,
		time.Now().UnixNano(), string(kind), source, message)
	return err
}

// Query returns events whose timestamp is in [from, to] inclusive, ordered by
// timestamp ascending. When one or more kinds are given, only events of those
// kinds are returned; with no kinds, every kind is included.
func (l *Log) Query(from, to time.Time, kinds ...Kind) ([]Event, error) {
	query := `SELECT ts, kind, source, message FROM events WHERE ts BETWEEN ? AND ?`
	args := []any{from.UnixNano(), to.UnixNano()}
	if len(kinds) > 0 {
		placeholders := make([]string, len(kinds))
		for i, k := range kinds {
			placeholders[i] = "?"
			args = append(args, string(k))
		}
		query += ` AND kind IN (` + strings.Join(placeholders, ", ") + `)`
	}
	query += ` ORDER BY ts ASC`

	rows, err := l.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var (
			ts              int64
			kind            string
			source, message string
		)
		if err := rows.Scan(&ts, &kind, &source, &message); err != nil {
			return nil, err
		}
		out = append(out, Event{
			TS:      time.Unix(0, ts),
			Kind:    Kind(kind),
			Source:  source,
			Message: message,
		})
	}
	return out, rows.Err()
}

// Close releases the underlying database handle.
func (l *Log) Close() error {
	return l.db.Close()
}
