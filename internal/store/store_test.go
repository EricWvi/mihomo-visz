package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistenceAggregationAndFilters(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "data")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	u := Usage{Hour: at.Unix(), Node: "深港专线1", Host: "www.google.com", Domain: "google.com", Upload: 100, Download: 200}
	if err = s.Record(ctx, []Usage{u}, []Observation{{Hour: at.Unix(), Upload: 100, Download: 220}}, at); err != nil {
		t.Fatal(err)
	}
	if err = s.Record(ctx, []Usage{u, {Hour: at.Unix(), Node: "深港专线1", Host: "api.google.com", Domain: "google.com", Upload: 10, Download: 20}}, nil, at.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err = s.RecordGap(ctx, at, at.Add(time.Second), "测试缺口"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	f := Filter{From: at.Add(-12 * time.Hour), To: at.Add(12 * time.Hour), Group: "domain"}
	v, err := s.Traffic(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Rows) != 1 || v.Rows[0].Upload != 210 || v.Rows[0].Download != 420 || v.GlobalDownload != 220 || len(v.Nodes) != 1 || len(v.Gaps) != 1 {
		t.Fatalf("持久化或聚合错误: %+v", v)
	}
	f.Group = "host"
	v, err = s.Traffic(ctx, f)
	if err != nil || len(v.Rows) != 2 {
		t.Fatalf("完整域名分组: %+v %v", v, err)
	}
	f.Search = "GOOGLE.COM"
	v, err = s.Traffic(ctx, f)
	if err != nil || len(v.Rows) != 2 {
		t.Fatalf("域名搜索: %+v %v", v, err)
	}
	f.Search = "%"
	v, err = s.Traffic(ctx, f)
	if err != nil || len(v.Rows) != 0 {
		t.Fatalf("百分号应按普通字符搜索: %+v %v", v, err)
	}
	f.Search = ""
	f.Node = "不存在"
	v, err = s.Traffic(ctx, f)
	if err != nil || len(v.Rows) != 0 || len(v.Nodes) != 1 {
		t.Fatalf("节点筛选: %+v %v", v, err)
	}
	last, err := s.LastSample(ctx)
	if err != nil || !last.Equal(at.Add(time.Second)) {
		t.Fatalf("checkpoint: %v %v", last, err)
	}
}
func TestRecordRollsBackWholeSnapshot(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	now := time.Now().Truncate(time.Hour)
	err = s.Record(ctx, []Usage{{Hour: now.Unix(), Node: "n", Host: "h", Domain: "h", Upload: 1}, {Hour: now.Unix(), Node: "n", Host: "bad", Domain: "bad", Upload: -1}}, nil, now)
	if err == nil {
		t.Fatal("负数应被数据库拒绝")
	}
	v, err := s.Traffic(ctx, Filter{From: now, To: now.Add(time.Hour)})
	if err != nil || len(v.Rows) != 0 {
		t.Fatalf("事务没有回滚: %+v %v", v, err)
	}
	last, err := s.LastSample(ctx)
	if err != nil || !last.IsZero() {
		t.Fatalf("失败事务推进了检查点: %v %v", last, err)
	}
}

func TestRetentionExpiresUsageObservationsAndGapsTogether(t *testing.T) {
	db, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	cutoff := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	before := cutoff.Add(-time.Hour)
	after := cutoff.Add(time.Hour)
	batch := Batch{At: after, Usage: []Usage{
		{Hour: before.Unix(), Node: "old", Host: "old.com", Domain: "old.com", Download: 1},
		{Hour: cutoff.Unix(), Node: "boundary", Host: "boundary.com", Domain: "boundary.com", Download: 2},
		{Hour: after.Unix(), Node: "new", Host: "new.com", Domain: "new.com", Download: 3},
	}, Observations: []Observation{
		{Hour: before.Unix(), Download: 1}, {Hour: cutoff.Unix(), Download: 2}, {Hour: after.Unix(), Download: 3},
	}, Gaps: []Gap{
		{Start: before.Add(-time.Hour).UnixMilli(), End: before.UnixMilli(), Reason: "old"},
		{Start: before.UnixMilli(), End: after.UnixMilli(), Reason: "overlap"},
		{Start: after.UnixMilli(), End: after.Add(time.Minute).UnixMilli(), Reason: "new"},
	}}
	if err = db.Commit(ctx, batch, cutoff); err != nil {
		t.Fatal(err)
	}
	f := Filter{From: before.Add(-time.Hour), To: after.Add(time.Hour)}
	got, err := db.Traffic(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 2 || got.AllDownload != 5 || got.GlobalDownload != 5 || len(got.Gaps) != 2 {
		t.Fatalf("保留窗口错误: %+v", got)
	}
	for _, r := range got.Rows {
		if r.Hour < cutoff.Unix() {
			t.Fatalf("旧流量没有删除: %+v", r)
		}
	}
	for _, g := range got.Gaps {
		if g.Start < cutoff.UnixMilli() {
			t.Fatalf("跨窗口缺口没有裁剪: %+v", g)
		}
	}
	last, err := db.LastSample(ctx)
	if err != nil || !last.Equal(after) {
		t.Fatalf("检查点未原子保存: %v %v", last, err)
	}
	if err = db.Commit(ctx, Batch{}, cutoff); err != nil {
		t.Fatal(err)
	}
	again, err := db.Traffic(ctx, f)
	if err != nil || again.AllDownload != 5 || len(again.Gaps) != 2 {
		t.Fatalf("重复清理改变了保留的数据: %+v %v", again, err)
	}
	last, err = db.LastSample(ctx)
	if err != nil || !last.Equal(after) {
		t.Fatal("空批清理不应推进检查点")
	}
}
