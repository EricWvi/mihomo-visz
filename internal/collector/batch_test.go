package collector

import (
	"context"
	"testing"
	"time"

	"mihomo-visz/internal/config"
	"mihomo-visz/internal/store"
)

func TestBufferedDeltasCoalesceAndFailedFlushCanRetry(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	c := New(nil, db, config.Config{Location: time.UTC})
	now := time.Now().UTC()
	hour := now.Truncate(time.Hour)
	u := store.Usage{Hour: hour.Unix(), Node: "node", Host: "www.google.com", Domain: "google.com", Upload: 10, Download: 100}
	o := store.Observation{Hour: hour.Unix(), Upload: 10, Download: 110}
	c.pending.add([]store.Usage{u}, []store.Observation{o}, nil, now)
	c.pending.add([]store.Usage{u}, []store.Observation{o}, nil, now.Add(time.Second))
	if len(c.pending.usage) != 1 || len(c.pending.observations) != 1 {
		t.Fatal("同一小时节点域名没有在内存中合并")
	}
	f := store.Filter{From: hour, To: hour.Add(time.Hour)}
	before, err := db.Traffic(context.Background(), f)
	if err != nil || len(before.Rows) != 0 {
		t.Fatalf("提前落盘: %+v %v", before, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if err = c.flush(canceled, now); err == nil {
		t.Fatal("取消的落盘应该失败")
	}
	if len(c.pending.usage) != 1 || c.pending.at.IsZero() || c.Status().StorageError == "" {
		t.Fatal("失败后丢弃了缓冲数据或没有报告失败")
	}
	if err = c.flush(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	got, err := db.Traffic(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 1 || got.Rows[0].Upload != 20 || got.Rows[0].Download != 200 || got.GlobalDownload != 220 {
		t.Fatalf("重试重复计算或漏计: %+v", got)
	}
	if !c.pending.at.IsZero() || len(c.pending.usage) != 0 || c.Status().StorageError != "" {
		t.Fatal("成功落盘后没有清空缓冲或错误")
	}
	c.pending.add([]store.Usage{u}, []store.Observation{o}, nil, now.Add(2*time.Second))
	if err = c.flush(context.Background(), now.Add(30*time.Second)); err != nil {
		t.Fatal(err)
	}
	got, err = db.Traffic(context.Background(), f)
	if err != nil || got.Rows[0].Upload != 30 || got.Rows[0].Download != 300 {
		t.Fatalf("下一批数据错误: %+v %v", got, err)
	}
}
