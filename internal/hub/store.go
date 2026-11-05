// Package hub receives samples from agents, stores their history in SQLite
// and serves it over HTTP.
package hub

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/retransmit/kite/internal/protocol"
	_ "modernc.org/sqlite"
)

// ErrUnknownMachine is returned for a machine name the hub has never seen.
var ErrUnknownMachine = errors.New("unknown machine")

const schema = `
CREATE TABLE IF NOT EXISTS machines(id INTEGER PRIMARY KEY, name TEXT NOT NULL UNIQUE, os TEXT NOT NULL, interval_sec REAL NOT NULL, last_seen INTEGER NOT NULL, last_sample TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS series(id INTEGER PRIMARY KEY, machine_id INTEGER NOT NULL REFERENCES machines(id) ON DELETE CASCADE, key TEXT NOT NULL, UNIQUE(machine_id, key));
CREATE TABLE IF NOT EXISTS raw(series_id INTEGER NOT NULL, ts INTEGER NOT NULL, v REAL NOT NULL, PRIMARY KEY(series_id, ts)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS rollup_1m(series_id INTEGER NOT NULL, ts INTEGER NOT NULL, avg REAL NOT NULL, max REAL NOT NULL, PRIMARY KEY(series_id, ts)) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS meta(k TEXT PRIMARY KEY, v BLOB NOT NULL);
`

// Store holds machines and their metric history.
type Store struct {
	db *sql.DB

	mu     sync.Mutex
	series map[string]map[string]int64 // machine name -> key -> series id
}

// OpenStore opens (creating if needed) the SQLite database at path.
func OpenStore(path string) (*Store, error) {
	dsn := "file:" + path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	return &Store{db: db, series: map[string]map[string]int64{}}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Ingest records a sample at receivedAt (the hub's clock, never the agent's).
func (s *Store) Ingest(ctx context.Context, smp *protocol.Sample, receivedAt time.Time) error {
	latest, err := json.Marshal(smp)
	if err != nil {
		return err
	}
	ts := receivedAt.Unix()

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var mid int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO machines(name, os, interval_sec, last_seen, last_sample) VALUES(?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET os=excluded.os, interval_sec=excluded.interval_sec,
			last_seen=excluded.last_seen, last_sample=excluded.last_sample
		RETURNING id`, smp.Machine, smp.OS, smp.IntervalSec, ts, string(latest)).Scan(&mid)
	if err != nil {
		return fmt.Errorf("upsert machine: %w", err)
	}

	cache := s.series[smp.Machine]
	if cache == nil {
		cache = map[string]int64{}
	}
	insert, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO raw(series_id, ts, v) VALUES(?, ?, ?)`)
	if err != nil {
		return err
	}
	defer insert.Close()
	added := map[string]int64{}
	for _, p := range protocol.Flatten(smp) {
		id, ok := cache[p.Key]
		if !ok {
			if _, err := tx.ExecContext(ctx, `INSERT INTO series(machine_id, key) VALUES(?, ?) ON CONFLICT DO NOTHING`, mid, p.Key); err != nil {
				return err
			}
			if err := tx.QueryRowContext(ctx, `SELECT id FROM series WHERE machine_id=? AND key=?`, mid, p.Key).Scan(&id); err != nil {
				return err
			}
			added[p.Key] = id
		}
		if _, err := insert.ExecContext(ctx, id, ts, p.Value); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	// Only cache ids once the transaction that created them is committed.
	for k, id := range added {
		cache[k] = id
	}
	s.series[smp.Machine] = cache
	return nil
}

// MachineInfo describes a machine and its most recent sample.
type MachineInfo struct {
	Name        string
	OS          string
	IntervalSec float64
	LastSeen    time.Time
	Latest      json.RawMessage
}

// Machines lists every known machine, sorted by name.
func (s *Store) Machines(ctx context.Context) ([]MachineInfo, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, os, interval_sec, last_seen, last_sample FROM machines ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MachineInfo
	for rows.Next() {
		var m MachineInfo
		var seen int64
		var latest string
		if err := rows.Scan(&m.Name, &m.OS, &m.IntervalSec, &seen, &latest); err != nil {
			return nil, err
		}
		m.LastSeen = time.Unix(seen, 0)
		m.Latest = json.RawMessage(latest)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) machineID(ctx context.Context, name string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM machines WHERE name=?`, name).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrUnknownMachine
	}
	return id, err
}

// Keys lists the metric keys recorded for a machine, sorted.
func (s *Store) Keys(ctx context.Context, machine string) ([]string, error) {
	mid, err := s.machineID(ctx, machine)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT key FROM series WHERE machine_id=? ORDER BY key`, mid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// Resolution selects raw samples or 1-minute rollups.
type Resolution string

const (
	ResRaw Resolution = "raw"
	Res1m  Resolution = "1m"
)

// SeriesPoint is one point of a series. For raw data Avg and Max are equal.
type SeriesPoint struct {
	TS  int64
	Avg float64
	Max float64
}

// Query returns points for each key between from and to (inclusive),
// oldest first. Keys with no data map to an empty slice.
func (s *Store) Query(ctx context.Context, machine string, keys []string, from, to time.Time, res Resolution) (map[string][]SeriesPoint, error) {
	mid, err := s.machineID(ctx, machine)
	if err != nil {
		return nil, err
	}
	q := `SELECT r.ts, r.v, r.v FROM raw r JOIN series s ON s.id=r.series_id WHERE s.machine_id=? AND s.key=? AND r.ts BETWEEN ? AND ? ORDER BY r.ts`
	if res == Res1m {
		q = `SELECT r.ts, r.avg, r.max FROM rollup_1m r JOIN series s ON s.id=r.series_id WHERE s.machine_id=? AND s.key=? AND r.ts BETWEEN ? AND ? ORDER BY r.ts`
	}
	out := make(map[string][]SeriesPoint, len(keys))
	for _, k := range keys {
		rows, err := s.db.QueryContext(ctx, q, mid, k, from.Unix(), to.Unix())
		if err != nil {
			return nil, err
		}
		pts := []SeriesPoint{}
		for rows.Next() {
			var p SeriesPoint
			if err := rows.Scan(&p.TS, &p.Avg, &p.Max); err != nil {
				rows.Close()
				return nil, err
			}
			pts = append(pts, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
		out[k] = pts
	}
	return out, nil
}

// DeleteMachine removes a machine and all of its history. It reports
// whether the machine existed.
func (s *Store) DeleteMachine(ctx context.Context, name string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var mid int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM machines WHERE name=?`, name).Scan(&mid)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, q := range []string{
		`DELETE FROM raw WHERE series_id IN (SELECT id FROM series WHERE machine_id=?)`,
		`DELETE FROM rollup_1m WHERE series_id IN (SELECT id FROM series WHERE machine_id=?)`,
		`DELETE FROM series WHERE machine_id=?`,
		`DELETE FROM machines WHERE id=?`,
	} {
		if _, err := tx.ExecContext(ctx, q, mid); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	delete(s.series, name)
	return true, nil
}

func (s *Store) getMeta(ctx context.Context, k string) ([]byte, bool, error) {
	var v []byte
	err := s.db.QueryRowContext(ctx, `SELECT v FROM meta WHERE k=?`, k).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	return v, err == nil, err
}

func (s *Store) setMeta(ctx context.Context, k string, v []byte) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO meta(k, v) VALUES(?, ?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}

// Secret returns the hub's persistent 32-byte secret, creating it once.
func (s *Store) Secret(ctx context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v, ok, err := s.getMeta(ctx, "session_secret"); err != nil || ok {
		return v, err
	}
	v := make([]byte, 32)
	if _, err := rand.Read(v); err != nil {
		return nil, err
	}
	return v, s.setMeta(ctx, "session_secret", v)
}

func (s *Store) machineInterval(ctx context.Context, name string) (float64, error) {
	var v float64
	err := s.db.QueryRowContext(ctx, `SELECT interval_sec FROM machines WHERE name=?`, name).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrUnknownMachine
	}
	return v, err
}
