// Package sqlite adapts typed points to a small SQLite store. GUI code need not import it.
package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/uk0/silk/core"
	"github.com/uk0/silk/point"
)

type StoredSample struct {
	Name  string
	Value core.Value
}

// Store is a small typed SQLite point store, not a high-throughput historian.
// Publish commits latest + history together. All errors reach the caller; no
// best-effort dropping. It is safe for concurrent callers; Close follows workers.
type Store struct{ db *sql.DB }

// LoadSample is the optional runtime restore seam, independent of SQL types.
func (s *Store) LoadSample(ctx context.Context, name string) (core.Value, bool, error) {
	v, err := s.Latest(ctx, name)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Value{}, false, nil
	}
	return v.Value, err == nil, err
}

func Open(path string) (*Store, error) {
	if !StoreAvailable() {
		return nil, fmt.Errorf("point: SQLite point storage requires cgo")
	}
	if path == "" {
		return nil, fmt.Errorf("point: empty database path")
	}
	db, e := sql.Open("sqlite3", path)
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS point_definitions(name TEXT PRIMARY KEY,kind TEXT NOT NULL,category TEXT NOT NULL,unit TEXT NOT NULL,description TEXT NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS point_latest(name TEXT PRIMARY KEY,payload TEXT NOT NULL,quality INTEGER NOT NULL,ts INTEGER NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS point_history(seq INTEGER PRIMARY KEY AUTOINCREMENT,name TEXT NOT NULL,payload TEXT NOT NULL,quality INTEGER NOT NULL,ts INTEGER NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS point_history_time ON point_history(name,ts,seq)`,
		`CREATE INDEX IF NOT EXISTS point_history_retention ON point_history(ts,seq)`,
	} {
		if _, e = db.Exec(stmt); e != nil {
			db.Close()
			return nil, e
		}
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func (s *Store) Define(ctx context.Context, d point.Definition) error {
	if e := d.Validate(); e != nil {
		return e
	}
	// Conditional UPSERT keeps kind immutable even under concurrent definitions.
	r, e := s.db.ExecContext(ctx, `INSERT INTO point_definitions VALUES(?,?,?,?,?) ON CONFLICT(name) DO UPDATE SET category=excluded.category,unit=excluded.unit,description=excluded.description WHERE kind=excluded.kind`, d.Name, d.Kind, d.Category, d.Unit, d.Description)
	if e != nil {
		return e
	}
	n, e := r.RowsAffected()
	if e != nil {
		return e
	}
	if n == 0 {
		return fmt.Errorf("point: existing point type differs")
	}
	return nil
}
func (s *Store) Definitions(ctx context.Context) ([]point.Definition, error) {
	rows, e := s.db.QueryContext(ctx, `SELECT name,kind,category,unit,description FROM point_definitions ORDER BY name`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []point.Definition{}
	for rows.Next() {
		var d point.Definition
		if e = rows.Scan(&d.Name, &d.Kind, &d.Category, &d.Unit, &d.Description); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// Publish stores one observation, including unchanged values and quality/time.
// Nil is only allowed for Bad/Uncertain samples before a first successful read.
func (s *Store) Publish(ctx context.Context, name string, v core.Value) error {
	if v.Quality > core.QualityGood || v.Time.IsZero() {
		return fmt.Errorf("point: invalid quality or missing timestamp")
	}
	// UnixNano is only reversible inside its signed nanosecond range.
	ns := v.Time.UnixNano()
	if !time.Unix(0, ns).Equal(v.Time) {
		return fmt.Errorf("point: timestamp out of range")
	}
	tx, e := s.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var kind point.Kind
	if e = tx.QueryRowContext(ctx, `SELECT kind FROM point_definitions WHERE name=?`, name).Scan(&kind); e != nil {
		return e
	}
	var raw any
	if v.Raw != nil || v.Quality == core.QualityGood {
		raw, e = point.Convert(v.Raw, kind)
		if e != nil {
			return e
		}
	}
	if text, ok := raw.(string); ok && len(text) > 1<<20 {
		return fmt.Errorf("point: string sample exceeds 1 MiB")
	}
	data, e := json.Marshal(raw)
	if e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO point_latest VALUES(?,?,?,?) ON CONFLICT(name) DO UPDATE SET payload=excluded.payload,quality=excluded.quality,ts=excluded.ts`, name, string(data), v.Quality, ns); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO point_history(name,payload,quality,ts) VALUES(?,?,?,?)`, name, string(data), v.Quality, ns); e != nil {
		return e
	}
	return tx.Commit()
}
func decodeSample(name, payload string, q int, ts int64, kind point.Kind) (StoredSample, error) {
	d := json.NewDecoder(strings.NewReader(payload))
	d.UseNumber()
	var raw any
	e := d.Decode(&raw)
	if e != nil {
		return StoredSample{}, e
	}
	if raw != nil {
		raw, e = point.Convert(raw, kind)
		if e != nil {
			return StoredSample{}, e
		}
	}
	if q < 0 || q > int(core.QualityGood) {
		return StoredSample{}, fmt.Errorf("point: corrupted quality")
	}
	return StoredSample{name, core.Value{Raw: raw, Quality: core.Quality(q), Time: time.Unix(0, ts).UTC()}}, nil
}
func (s *Store) Latest(ctx context.Context, name string) (StoredSample, error) {
	var p string
	var q int
	var ts int64
	var kind point.Kind
	e := s.db.QueryRowContext(ctx, `SELECT l.payload,l.quality,l.ts,d.kind FROM point_latest l JOIN point_definitions d ON d.name=l.name WHERE l.name=?`, name).Scan(&p, &q, &ts, &kind)
	if e != nil {
		return StoredSample{}, e
	}
	return decodeSample(name, p, q, ts, kind)
}

// History is bounded, ordered by timestamp then insertion order. No implicit
// deletion: retention/compaction policies belong to the application in this version.
func (s *Store) History(ctx context.Context, name string, from, to time.Time, limit int) ([]StoredSample, error) {
	if from.After(to) || limit < 1 || limit > 10000 || !time.Unix(0, from.UnixNano()).Equal(from) || !time.Unix(0, to.UnixNano()).Equal(to) {
		return nil, fmt.Errorf("point: invalid history range/limit")
	}
	rows, e := s.db.QueryContext(ctx, `SELECT h.payload,h.quality,h.ts,d.kind FROM point_history h JOIN point_definitions d ON d.name=h.name WHERE h.name=? AND h.ts BETWEEN ? AND ? ORDER BY h.ts,h.seq LIMIT ?`, name, from.UnixNano(), to.UnixNano(), limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []StoredSample{}
	bytes := 0
	for rows.Next() {
		var p string
		var q int
		var ts int64
		var k point.Kind
		if e = rows.Scan(&p, &q, &ts, &k); e != nil {
			return nil, e
		}
		bytes += len(p)
		if bytes > 16<<20 {
			return nil, fmt.Errorf("point: history exceeds 16 MiB payload budget")
		}
		v, e := decodeSample(name, p, q, ts, k)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Prune deletes at most limit old history rows; definitions/latest remain intact.
func (s *Store) Prune(ctx context.Context, before time.Time, limit int) (int64, error) {
	if limit < 1 || limit > 10000 || !time.Unix(0, before.UnixNano()).Equal(before) {
		return 0, fmt.Errorf("point: invalid retention cutoff/budget")
	}
	result, e := s.db.ExecContext(ctx, `DELETE FROM point_history WHERE seq IN (SELECT seq FROM point_history WHERE ts<? ORDER BY ts,seq LIMIT ?)`, before.UnixNano(), limit)
	if e != nil {
		return 0, e
	}
	return result.RowsAffected()
}
