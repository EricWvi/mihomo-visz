package collector

import (
	"testing"
	"time"

	"mihomo-visz/internal/mihomo"
)

func at(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}
func TestDiffExistingNewAndInternal(t *testing.T) {
	from := at("2026-11-01T12:00:00+08:00")
	to := from.Add(time.Second)
	existing := mihomo.Connection{ID: "existing", Start: from.Add(-time.Hour), Metadata: mihomo.Metadata{Host: "www.google.com"}, Chains: []string{"深港专线1", "自动选择"}, Upload: 100, Download: 1000}
	current := existing
	current.Upload += 10
	current.Download += 100
	fresh := mihomo.Connection{ID: "fresh", Start: from.Add(500 * time.Millisecond), Metadata: mihomo.Metadata{Host: "apple.com"}, Chains: []string{"深港专线1", "自动选择"}, Upload: 2, Download: 20}
	inner := fresh
	inner.ID = "inner"
	inner.Metadata.Type = "Inner"
	inner.Upload = 99999
	inner.Download = 99999
	old := fresh
	old.ID = "unseen-old"
	old.Start = from.Add(-time.Hour)
	old.Download = 99999
	prev := mihomo.Snapshot{Connections: []mihomo.Connection{existing}, UploadTotal: 100, DownloadTotal: 1000}
	snap := mihomo.Snapshot{Connections: []mihomo.Connection{current, fresh, inner, old, current}, UploadTotal: 112, DownloadTotal: 1120}
	ps := map[string]mihomo.Proxy{"深港专线1": {Type: "Trojan"}, "自动选择": {Type: "Selector"}}
	rows, obs := diff(prev, snap, from, to, ps, time.FixedZone("CST", 8*3600))
	if len(rows) != 2 || len(obs) != 1 {
		t.Fatalf("rows=%+v observations=%+v", rows, obs)
	}
	if rows[0].Upload != 10 || rows[0].Download != 100 || rows[0].Domain != "google.com" || rows[0].Node != "深港专线1" {
		t.Fatalf("已有连接差分错误: %+v", rows[0])
	}
	if rows[1].Upload != 2 || rows[1].Download != 20 {
		t.Fatalf("新连接初始值错误: %+v", rows[1])
	}
	if obs[0].Upload != 12 || obs[0].Download != 120 {
		t.Fatalf("全局差分错误: %+v", obs)
	}
}
func TestSplitAcrossHourConservesBytes(t *testing.T) {
	loc := time.FixedZone("CST", 8*3600)
	parts := split(at("2026-11-01T12:59:59+08:00"), at("2026-11-01T13:00:01+08:00"), 101, 201, loc)
	if len(parts) != 2 || parts[0].upload != 50 || parts[1].upload != 51 || parts[0].download+parts[1].download != 201 {
		t.Fatalf("跨小时分配错误: %+v", parts)
	}
	if time.Unix(parts[0].hour, 0).In(loc).Hour() != 12 || time.Unix(parts[1].hour, 0).In(loc).Hour() != 13 {
		t.Fatalf("时间桶错误: %+v", parts)
	}
}
func TestHourStartHalfHourTimezone(t *testing.T) {
	loc := time.FixedZone("IST", 19800)
	in := at("2026-11-01T12:30:30+05:30")
	if got := hourStart(in, loc).Format(time.RFC3339); got != "2026-11-01T12:00:00+05:30" {
		t.Fatal(got)
	}
}
func TestDomainUsesPublicSuffixAndKeepsUnknown(t *testing.T) {
	for _, tc := range []struct {
		m          mihomo.Metadata
		host, root string
	}{
		{mihomo.Metadata{Host: "WWW.Example.CO.UK."}, "www.example.co.uk", "example.co.uk"},
		{mihomo.Metadata{Host: "front.example", SniffHost: "api.google.com"}, "api.google.com", "google.com"},
		{mihomo.Metadata{DestinationIP: "1.1.1.1"}, "1.1.1.1", "未识别域名"},
		{mihomo.Metadata{}, "未知地址", "未识别域名"},
	} {
		host, root := domain(tc.m)
		if host != tc.host || root != tc.root {
			t.Errorf("got (%s,%s) want (%s,%s)", host, root, tc.host, tc.root)
		}
	}
}
func TestCounterRollbackDoesNotCreateNegativeUsage(t *testing.T) {
	from := at("2026-11-01T12:00:00Z")
	p := mihomo.Connection{ID: "same", Upload: 100, Download: 200}
	c := p
	c.Download = 0
	rows, obs := diff(mihomo.Snapshot{Connections: []mihomo.Connection{p}, UploadTotal: 100}, mihomo.Snapshot{Connections: []mihomo.Connection{c}, UploadTotal: 10}, from, from.Add(time.Second), nil, time.UTC)
	if len(rows) != 0 || len(obs) != 0 {
		t.Fatalf("计数回退被计算: %+v %+v", rows, obs)
	}
}
