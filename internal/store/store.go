// Package store persists observed deltas rather than connection lifetime totals.
package store

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }
type Usage struct {
	Hour     int64  `json:"hour"`
	Node     string `json:"node"`
	Host     string `json:"host"`
	Domain   string `json:"domain"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}
type Observation struct{ Hour, Upload, Download int64 }
type Batch struct {
	Usage        []Usage
	Observations []Observation
	Gaps         []Gap
	At           time.Time
}
type Gap struct {
	Start  int64  `json:"start"`
	End    int64  `json:"end"`
	Reason string `json:"reason"`
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path, err := filepath.Abs(filepath.Join(dir, "traffic.db"))
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	q := url.Values{}
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "foreign_keys(1)")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
 CREATE TABLE IF NOT EXISTS usage (
 hour INTEGER NOT NULL,node TEXT NOT NULL,host TEXT NOT NULL,domain TEXT NOT NULL,
 upload INTEGER NOT NULL CHECK(upload>=0),download INTEGER NOT NULL CHECK(download>=0),
 PRIMARY KEY(hour,node,host)
 ) WITHOUT ROWID;
 CREATE TABLE IF NOT EXISTS observations (
 hour INTEGER PRIMARY KEY,upload INTEGER NOT NULL,download INTEGER NOT NULL
 );
 CREATE TABLE IF NOT EXISTS gaps (
 id INTEGER PRIMARY KEY,start INTEGER NOT NULL,end INTEGER NOT NULL,reason TEXT NOT NULL
 );
 CREATE INDEX IF NOT EXISTS gaps_end ON gaps(end);
 CREATE TABLE IF NOT EXISTS checkpoint (
 id INTEGER PRIMARY KEY CHECK(id=1),last_sample INTEGER NOT NULL
 );
 PRAGMA user_version=1;
 `)
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db}, nil
}
func (s *Store) Close() error { return s.db.Close() }

// Commit atomically persists a coalesced batch, gaps and its sample checkpoint,
// then expires history. A failed transaction leaves the caller's batch intact.
func (s *Store) Commit(ctx context.Context, batch Batch, cutoff time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if len(batch.Usage) > 0 {
		stmt, err := tx.PrepareContext(ctx, `INSERT INTO usage(hour,node,host,domain,upload,download) VALUES(?,?,?,?,?,?)
 ON CONFLICT(hour,node,host) DO UPDATE SET upload=upload+excluded.upload,download=download+excluded.download`)
		if err != nil {
			return err
		}
		defer stmt.Close()
		for _, u := range batch.Usage {
			if u.Upload == 0 && u.Download == 0 {
				continue
			}
			if _, err = stmt.ExecContext(ctx, u.Hour, u.Node, u.Host, u.Domain, u.Upload, u.Download); err != nil {
				return err
			}
		}
	}
	for _, o := range batch.Observations {
		if _, err = tx.ExecContext(ctx, `INSERT INTO observations(hour,upload,download) VALUES(?,?,?) ON CONFLICT(hour) DO UPDATE SET upload=upload+excluded.upload,download=download+excluded.download`, o.Hour, o.Upload, o.Download); err != nil {
			return err
		}
	}
	for _, g := range batch.Gaps {
		if g.End > g.Start && g.Start > 0 {
			if _, err = tx.ExecContext(ctx, `INSERT INTO gaps(start,end,reason) VALUES(?,?,?)`, g.Start, g.End, g.Reason); err != nil {
				return err
			}
		}
	}
	if !batch.At.IsZero() {
		if _, err = tx.ExecContext(ctx, `INSERT INTO checkpoint(id,last_sample) VALUES(1,?) ON CONFLICT(id) DO UPDATE SET last_sample=excluded.last_sample`, batch.At.UnixMilli()); err != nil {
			return err
		}
	}
	if !cutoff.IsZero() {
		// Usage is hourly: drop the entire boundary bucket when its start expires.
		// This guarantees no data older than the rolling retention window survives.
		for _, table := range []string{"usage", "observations"} {
			if _, err = tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE hour < ?", cutoff.Unix()); err != nil {
				return err
			}
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM gaps WHERE end <= ?", cutoff.UnixMilli()); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE gaps SET start = ? WHERE start < ?", cutoff.UnixMilli(), cutoff.UnixMilli()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Record(ctx context.Context, usage []Usage, observations []Observation, at time.Time) error {
	return s.Commit(ctx, Batch{Usage: usage, Observations: observations, At: at}, time.Time{})
}
func (s *Store) LastSample(ctx context.Context) (time.Time, error) {
	var ms int64
	err := s.db.QueryRowContext(ctx, "SELECT last_sample FROM checkpoint WHERE id=1").Scan(&ms)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.UnixMilli(ms), nil
}
func (s *Store) RecordGap(ctx context.Context, from, to time.Time, reason string) error {
	if from.IsZero() || !to.After(from) {
		return nil
	}
	return s.Commit(ctx, Batch{Gaps: []Gap{{Start: from.UnixMilli(), End: to.UnixMilli(), Reason: reason}}}, time.Time{})
}

type Filter struct {
	From, To     time.Time
	Node, Search string
	Group        string
}
type TrafficRow struct {
	Hour     int64  `json:"hour"`
	Node     string `json:"node"`
	Domain   string `json:"domain"`
	Upload   int64  `json:"upload"`
	Download int64  `json:"download"`
}
type Traffic struct {
	Rows           []TrafficRow `json:"rows"`
	Nodes          []string     `json:"nodes"`
	Gaps           []Gap        `json:"gaps"`
	GlobalUpload   int64        `json:"global_upload"`
	GlobalDownload int64        `json:"global_download"`
	AllUpload      int64        `json:"all_upload"`
	AllDownload    int64        `json:"all_download"`
}

func (s *Store) Traffic(ctx context.Context, f Filter) (Traffic, error) {
	result := Traffic{Rows: []TrafficRow{}, Nodes: []string{}, Gaps: []Gap{}}
	// Keep a consistent read view while the collector is committing new snapshots.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	column := "domain"
	if f.Group == "host" {
		column = "host"
	}
	query := fmt.Sprintf("SELECT hour,node,%s,SUM(upload),SUM(download) FROM usage WHERE hour>=? AND hour<?", column)
	args := []any{f.From.Unix(), f.To.Unix()}
	if f.Node != "" {
		query += " AND node=?"
		args = append(args, f.Node)
	}
	if f.Search != "" {
		query += " AND instr(lower(" + column + "),lower(?))>0"
		args = append(args, f.Search)
	}
	query += " GROUP BY hour,node," + column + " ORDER BY hour,node," + column
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var r TrafficRow
		if err = rows.Scan(&r.Hour, &r.Node, &r.Domain, &r.Upload, &r.Download); err != nil {
			rows.Close()
			return result, err
		}
		result.Rows = append(result.Rows, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT DISTINCT node FROM usage WHERE hour>=? AND hour<? ORDER BY node", f.From.Unix(), f.To.Unix())
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var n string
		if err = rows.Scan(&n); err != nil {
			rows.Close()
			return result, err
		}
		result.Nodes = append(result.Nodes, n)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	rows, err = tx.QueryContext(ctx, "SELECT start,end,reason FROM gaps WHERE start<? AND end>? ORDER BY start", f.To.UnixMilli(), f.From.UnixMilli())
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var g Gap
		if err = rows.Scan(&g.Start, &g.End, &g.Reason); err != nil {
			rows.Close()
			return result, err
		}
		result.Gaps = append(result.Gaps, g)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(upload),0),COALESCE(SUM(download),0) FROM observations WHERE hour>=? AND hour<?", f.From.Unix(), f.To.Unix()).Scan(&result.GlobalUpload, &result.GlobalDownload); err != nil {
		return result, err
	}
	if err = tx.QueryRowContext(ctx, "SELECT COALESCE(SUM(upload),0),COALESCE(SUM(download),0) FROM usage WHERE hour>=? AND hour<?", f.From.Unix(), f.To.Unix()).Scan(&result.AllUpload, &result.AllDownload); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
