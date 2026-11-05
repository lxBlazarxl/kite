package hub

import (
	"context"
	"database/sql"
	"encoding/binary"
	"time"
)

const (
	rollupChunk  = 1440 * 60 // seconds of raw data aggregated per transaction
	watermarkKey = "rollup_watermark"
)

// Rollup aggregates every complete minute since the last run into 1-minute
// avg/max rows. It is idempotent and catches up after downtime in
// day-sized chunks.
func (s *Store) Rollup(ctx context.Context, now time.Time) error {
	end := now.Unix() / 60 * 60
	s.mu.Lock()
	defer s.mu.Unlock()

	var w int64
	if v, ok, err := s.getMeta(ctx, watermarkKey); err != nil {
		return err
	} else if ok && len(v) == 8 {
		w = int64(binary.BigEndian.Uint64(v))
	} else {
		var first sql.NullInt64
		if err := s.db.QueryRowContext(ctx, `SELECT min(ts) FROM raw`).Scan(&first); err != nil {
			return err
		}
		if !first.Valid {
			return nil
		}
		w = first.Int64 / 60 * 60
	}

	for w < end {
		next := min(w+rollupChunk, end)
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `
			INSERT OR REPLACE INTO rollup_1m(series_id, ts, avg, max)
			SELECT series_id, ts/60*60, avg(v), max(v) FROM raw
			WHERE ts >= ? AND ts < ? GROUP BY series_id, ts/60`, w, next)
		if err == nil {
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, uint64(next))
			_, err = tx.ExecContext(ctx, `INSERT INTO meta(k, v) VALUES(?, ?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, watermarkKey, buf)
		}
		if err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		w = next
	}
	return nil
}

// Prune deletes raw rows older than rawRetention and rollups older than
// rollupRetention.
func (s *Store) Prune(ctx context.Context, now time.Time, rawRetention, rollupRetention time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM raw WHERE ts < ?`, now.Add(-rawRetention).Unix()); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM rollup_1m WHERE ts < ?`, now.Add(-rollupRetention).Unix())
	return err
}

// ChooseResolution picks raw data for short, recent ranges and 1-minute
// rollups otherwise.
func ChooseResolution(from, to, now time.Time, rawRetention time.Duration) Resolution {
	if to.Sub(from) <= 3*time.Hour && !from.Before(now.Add(-rawRetention)) {
		return ResRaw
	}
	return Res1m
}
