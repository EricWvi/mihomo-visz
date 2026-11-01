package collector

import (
	"context"
	"errors"
	"time"

	"mihomo-visz/internal/store"
)

type usageKey struct {
	hour       int64
	node, host string
}
type pendingBatch struct {
	usage        map[usageKey]store.Usage
	observations map[int64]store.Observation
	gaps         []store.Gap
	at           time.Time
}

func (b *pendingBatch) add(usage []store.Usage, observations []store.Observation, gap *store.Gap, at time.Time) {
	if b.usage == nil {
		b.usage = map[usageKey]store.Usage{}
	}
	if b.observations == nil {
		b.observations = map[int64]store.Observation{}
	}
	for _, u := range usage {
		key := usageKey{u.Hour, u.Node, u.Host}
		v := b.usage[key]
		v.Hour = u.Hour
		v.Node = u.Node
		v.Host = u.Host
		v.Domain = u.Domain
		v.Upload += u.Upload
		v.Download += u.Download
		b.usage[key] = v
	}
	for _, o := range observations {
		v := b.observations[o.Hour]
		v.Hour = o.Hour
		v.Upload += o.Upload
		v.Download += o.Download
		b.observations[o.Hour] = v
	}
	if gap != nil {
		b.gaps = append(b.gaps, *gap)
	}
	b.at = at
}
func (b *pendingBatch) snapshot() store.Batch {
	batch := store.Batch{At: b.at, Gaps: b.gaps}
	for _, u := range b.usage {
		batch.Usage = append(batch.Usage, u)
	}
	for _, o := range b.observations {
		batch.Observations = append(batch.Observations, o)
	}
	return batch
}

func (c *Collector) flush(ctx context.Context, now time.Time) error {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	cutoff := now.Add(-time.Duration(c.cfg.RetentionDays) * 24 * time.Hour)
	err := c.store.Commit(ctx, c.pending.snapshot(), cutoff)
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.status.StorageError = "保存流量失败，未落盘数据仍保留在内存中，请检查数据库和磁盘空间"
		return errors.New(c.status.StorageError)
	}
	c.pending = pendingBatch{}
	c.status.StorageError = ""
	c.status.LastFlush = now.UnixMilli()
	return nil
}
