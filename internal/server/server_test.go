package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mihomo-visz/internal/collector"
	"mihomo-visz/internal/config"
	"mihomo-visz/internal/store"
)

type fakeStatus struct{}

func (fakeStatus) Status() collector.Status {
	return collector.Status{Connected: true, Version: "test"}
}
func TestReadOnlyAPIAndHourlyData(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	loc := time.FixedZone("CST", 8*3600)
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, loc)
	if err = db.Record(context.Background(), []store.Usage{{Hour: at.Unix(), Node: "深港专线1", Host: "google.com", Domain: "google.com", Download: 1000}}, nil, at); err != nil {
		t.Fatal(err)
	}
	assets := t.TempDir()
	os.WriteFile(filepath.Join(assets, "index.html"), []byte("test frontend"), 0600)
	h := Handler(db, fakeStatus{}, config.Config{Location: loc, Timezone: "Asia/Shanghai", FrontendDir: assets})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/traffic?date=2026-11-01", nil))
	var body struct {
		store.Traffic
		Hours []int64 `json:"hours"`
	}
	if err = json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if w.Code != 200 || len(body.Hours) != 24 || len(body.Rows) != 1 || body.Rows[0].Download != 1000 {
		t.Fatalf("API 返回错误: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{"/api/traffic?date=invalid", "/api/traffic?date=2026-11-01&group=sql"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("参数未被拒绝 %s %d", path, w.Code)
		}
	}
	for _, method := range []string{"POST", "PUT", "PATCH", "DELETE"} {
		w = httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, "/api/status", nil))
		if w.Code != 405 {
			t.Fatalf("%s 得到 %d", method, w.Code)
		}
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/status", nil))
	if strings.Contains(w.Body.String(), "secret") {
		t.Fatal("API 暴露了密钥配置")
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Body.String() != "test frontend" {
		t.Fatal("静态文件未返回")
	}
}
