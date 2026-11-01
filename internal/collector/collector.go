package collector

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"mihomo-visz/internal/config"
	"mihomo-visz/internal/mihomo"
	"mihomo-visz/internal/store"
)

type Status struct {
	Connected       bool   `json:"connected"`
	LastFlush       int64  `json:"last_flush"`
	FlushIntervalMS int64  `json:"flush_interval_ms"`
	RetentionDays   int    `json:"retention_days"`
	StorageError    string `json:"storage_error"`
	Version         string `json:"version"`
	LastSample      int64  `json:"last_sample"`
	StartedAt       int64  `json:"started_at"`
	Active          int    `json:"active"`
	Inner           int    `json:"inner"`
	UnknownDomain   int    `json:"unknown_domain"`
	Samples         int64  `json:"samples"`
	Reconnects      int    `json:"reconnects"`
	Error           string `json:"error"`
	IntervalMS      int64  `json:"interval_ms"`
}
type Collector struct {
	client    *mihomo.Client
	store     *store.Store
	cfg       config.Config
	mu        sync.RWMutex
	status    Status
	pendingMu sync.Mutex
	pending   pendingBatch
}

func New(c *mihomo.Client, s *store.Store, cfg config.Config) *Collector {
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = config.DefaultFlushInterval
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = config.DefaultRetentionDays
	}
	return &Collector{client: c, store: s, cfg: cfg, status: Status{
		StartedAt: time.Now().UnixMilli(), IntervalMS: cfg.Interval.Milliseconds(),
		FlushIntervalMS: cfg.FlushInterval.Milliseconds(), RetentionDays: cfg.RetentionDays,
	}}
}
func (c *Collector) Status() Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s := c.status
	if s.LastSample == 0 || time.Since(time.UnixMilli(s.LastSample)) > c.cfg.StaleAfter {
		s.Connected = false
	}
	return s
}
func (c *Collector) Run(ctx context.Context) error {
	// Expire old records immediately, even if the upstream stays unavailable.
	if err := c.flush(ctx, time.Now()); err != nil {
		return err
	}
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		ticker := time.NewTicker(c.cfg.FlushInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := c.flush(ctx, now); err != nil && ctx.Err() == nil {
					slog.Error("批量落盘失败，将在下个周期重试", "reason", err)
				}
			}
		}
	}()
	c.collect(ctx)
	<-writerDone
	// Signal cancellation stops collection but must not cancel the final commit.
	flushCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return c.flush(flushCtx, time.Now())
}

func (c *Collector) collect(ctx context.Context) {
	delay := time.Second
	for ctx.Err() == nil {
		err := c.session(ctx)
		if ctx.Err() != nil {
			return
		}
		c.mu.Lock()
		c.status.Connected = false
		c.status.Reconnects++
		c.status.Error = err.Error()
		c.mu.Unlock()
		slog.Warn("采集中断，将自动重连", "reason", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		// A healthy session should retry promptly after an isolated interruption.
		if s := c.Status(); s.LastSample > 0 && time.Since(time.UnixMilli(s.LastSample)) < c.cfg.StaleAfter {
			delay = time.Second
		}
	}
}
func (c *Collector) session(ctx context.Context) error {
	version, err := c.client.Version(ctx)
	if err != nil {
		return err
	}
	proxies, err := c.client.Proxies(ctx)
	if err != nil {
		return err
	}
	stream, err := c.client.Stream(ctx, c.cfg.Interval)
	if err != nil {
		return err
	}
	defer stream.Close()
	c.mu.RLock()
	lastSample := c.status.LastSample
	c.mu.RUnlock()
	var lastGood time.Time
	if lastSample != 0 {
		lastGood = time.UnixMilli(lastSample)
	} else {
		lastGood, err = c.store.LastSample(ctx)
		if err != nil {
			return errors.New("无法读取采集检查点")
		}
	}
	var previous mihomo.Snapshot
	var lastAt time.Time
	for ctx.Err() == nil {
		readCtx, cancel := context.WithTimeout(ctx, c.cfg.StaleAfter)
		snapshot, err := stream.Read(readCtx)
		cancel()
		if err != nil {
			return err
		}
		now := time.Now()
		if snapshot.UploadTotal < 0 || snapshot.DownloadTotal < 0 {
			return errors.New("全局累计流量计数无效")
		}
		var usage []store.Usage
		var observations []store.Observation
		baseline := lastAt.IsZero()
		reason := "启动或断线重连，区间内流量未分配"
		if !baseline && (snapshot.UploadTotal < previous.UploadTotal || snapshot.DownloadTotal < previous.DownloadTotal) {
			baseline = true
			reason = "内核流量计数重置，重新建立基线"
		}
		if !baseline && now.Sub(lastAt) > c.cfg.StaleAfter {
			baseline = true
			reason = "快照间隔过长，重新建立基线"
		}
		var gap *store.Gap
		if baseline {
			if !lastGood.IsZero() && now.After(lastGood) {
				gap = &store.Gap{Start: lastGood.UnixMilli(), End: now.UnixMilli(), Reason: reason}
			}
		} else {
			usage, observations = diff(previous, snapshot, lastAt, now, proxies, c.cfg.Location)
		}
		c.pendingMu.Lock()
		c.pending.add(usage, observations, gap, now)
		c.pendingMu.Unlock()
		previous = snapshot
		lastAt = now
		lastGood = now
		active, inner, unknown := 0, 0, 0
		for _, conn := range snapshot.Connections {
			if isInner(conn) {
				inner++
				continue
			}
			active++
			_, root := domain(conn.Metadata)
			if root == "未识别域名" {
				unknown++
			}
		}
		c.mu.Lock()
		c.status.Connected = true
		c.status.Version = version
		c.status.LastSample = now.UnixMilli()
		c.status.Active = active
		c.status.Inner = inner
		c.status.UnknownDomain = unknown
		c.status.Samples++
		c.status.Error = ""
		c.mu.Unlock()
	}
	return ctx.Err()
}
