package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coder/websocket"
	"mihomo-visz/internal/config"
	"mihomo-visz/internal/mihomo"
	"mihomo-visz/internal/store"
)

func TestLiveSessionBaselinesAndCounterReset(t *testing.T) {
	snapshots, address := fakeController(t)
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cfg := config.Config{Location: time.UTC, Interval: time.Second, StaleAfter: 3 * time.Second}
	c := New(mihomo.New(address, func() (string, error) { return "", nil }), db, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { defer close(done); c.Run(ctx) }()
	start := time.Now().Add(-time.Minute)
	send := func(up, down int64, wantSamples int64) {
		snapshots <- mihomo.Snapshot{UploadTotal: up, DownloadTotal: down, Connections: []mihomo.Connection{{ID: "business", Start: start, Upload: up, Download: down, Chains: []string{"node"}, Metadata: mihomo.Metadata{Host: "google.com"}}}}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if c.Status().Samples >= wantSamples {
				return
			}
			time.Sleep(time.Millisecond * 5)
		}
		t.Fatalf("等待快照超时: %+v", c.Status())
	}
	send(100, 1000, 1)
	send(120, 1100, 2)
	send(0, 0, 3)
	send(10, 20, 4)
	// The default 30-second batch must not write on each received snapshot.
	if checkpoint, err := db.LastSample(context.Background()); err != nil || !checkpoint.IsZero() {
		t.Fatalf("采样时提前写入了检查点: %v %v", checkpoint, err)
	}
	c.pendingMu.Lock()
	buffered := c.pending.snapshot()
	c.pendingMu.Unlock()
	if len(buffered.Gaps) != 1 {
		t.Fatal("采集缺口没有进入同一批缓冲")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("采集器无法停止")
	}
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	v, err := db.Traffic(context.Background(), store.Filter{From: midnight.Add(-time.Hour), To: midnight.AddDate(0, 0, 1), Group: "domain"})
	if err != nil {
		t.Fatal(err)
	}
	var up, down int64
	for _, r := range v.Rows {
		up += r.Upload
		down += r.Download
	}
	if up != 30 || down != 120 {
		t.Fatalf("基线或重置重复计数: up=%d down=%d", up, down)
	}
	if len(v.Gaps) != 1 {
		t.Fatalf("未记录重置缺口: %+v", v.Gaps)
	}
}

func fakeController(t *testing.T) (chan mihomo.Snapshot, string) {
	t.Helper()
	snapshots := make(chan mihomo.Snapshot, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Error("采集器发出了写操作", r.Method)
			return
		}
		switch r.URL.Path {
		case "/version":
			w.Write([]byte(`{"version":"test"}`))
		case "/proxies":
			w.Write([]byte(`{"proxies":{"node":{"type":"Trojan"}}}`))
		case "/connections":
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				return
			}
			defer conn.CloseNow()
			for {
				select {
				case <-r.Context().Done():
					return
				case snapshot := <-snapshots:
					b, _ := json.Marshal(snapshot)
					if conn.Write(r.Context(), websocket.MessageText, b) != nil {
						return
					}
				}
			}
		}
	}))
	t.Cleanup(srv.Close)

	return snapshots, srv.URL
}

func TestPeriodicFlushDoesNotRequireAnotherSnapshot(t *testing.T) {
	snapshots, address := fakeController(t)
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := New(mihomo.New(address, func() (string, error) { return "", nil }), db, config.Config{
		Location: time.UTC, Interval: time.Second, StaleAfter: time.Minute, FlushInterval: 50 * time.Millisecond,
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(2 * time.Second):
			t.Error("采集器未停止")
		}
	}()
	start := time.Now().Add(-time.Minute)
	snapshots <- mihomo.Snapshot{UploadTotal: 100, Connections: []mihomo.Connection{{ID: "one", Start: start, Upload: 100, Chains: []string{"node"}, Metadata: mihomo.Metadata{Host: "google.com"}}}}
	deadline := time.Now().Add(2 * time.Second)
	for c.Status().Samples < 1 {
		if time.Now().After(deadline) {
			t.Fatal("基线未建立")
		}
		time.Sleep(time.Millisecond)
	}
	snapshots <- mihomo.Snapshot{UploadTotal: 120, Connections: []mihomo.Connection{{ID: "one", Start: start, Upload: 120, Chains: []string{"node"}, Metadata: mihomo.Metadata{Host: "google.com"}}}}
	for c.Status().Samples < 2 {
		if time.Now().After(deadline) {
			t.Fatal("第二次快照未到达")
		}
		time.Sleep(time.Millisecond)
	}
	// No third snapshot is sent: only the independent timer can persist the delta.
	for {
		now := time.Now().UTC()
		day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		traffic, err := db.Traffic(context.Background(), store.Filter{From: day.Add(-time.Hour), To: day.AddDate(0, 0, 1)})
		if err != nil {
			t.Fatal(err)
		}
		if traffic.AllUpload == 20 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("没有新快照时没有定时落盘: %+v", traffic)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if c.Status().Samples != 2 {
		t.Fatal("测试不应需要额外快照")
	}
}
