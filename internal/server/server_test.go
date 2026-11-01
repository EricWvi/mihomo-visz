package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
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
	h, err := Handler(db, fakeStatus{}, config.Config{Location: loc, Timezone: "Asia/Shanghai"}, fstest.MapFS{"index.html": {Data: []byte("test frontend")}})
	if err != nil {
		t.Fatal(err)
	}
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

func TestStaticCaching(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	web := fstest.MapFS{
		"index.html":           {Data: []byte("<html>app</html>")},
		"favicon.svg":          {Data: []byte("<svg/>")},
		"assets/index-abc.js":  {Data: []byte("console.log(1)")},
		"assets/index-abc.css": {Data: []byte("body{}")},
		".gitkeep":             {},
	}
	h, err := Handler(db, fakeStatus{}, config.Config{Location: time.UTC, Timezone: "UTC"}, web)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string, header ...string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		for i := 0; i+1 < len(header); i += 2 {
			r.Header.Set(header[i], header[i+1])
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	cases := []struct{ path, cache, typ string }{
		{"/", "no-cache", "text/html"},
		{"/favicon.svg", "no-cache", "image/svg+xml"},
		{"/assets/index-abc.js", "public, max-age=31536000, immutable", "javascript"},
		{"/assets/index-abc.css", "public, max-age=31536000, immutable", "text/css"},
	}
	for _, c := range cases {
		w := get(c.path)
		if w.Code != 200 || w.Header().Get("Cache-Control") != c.cache || !strings.Contains(w.Header().Get("Content-Type"), c.typ) || w.Header().Get("ETag") == "" {
			t.Fatalf("%s: %d cache=%q type=%q etag=%q", c.path, w.Code, w.Header().Get("Cache-Control"), w.Header().Get("Content-Type"), w.Header().Get("ETag"))
		}
		if again := get(c.path, "If-None-Match", w.Header().Get("ETag")); again.Code != http.StatusNotModified || again.Body.Len() != 0 {
			t.Fatalf("%s 未按 ETag 返回 304: %d", c.path, again.Code)
		}
	}
	if w := get("/some/page"); w.Code != 200 || w.Body.String() != "<html>app</html>" {
		t.Fatalf("页面路径未回退到 index.html: %d", w.Code)
	}
	for _, path := range []string{"/assets/missing.js", "/missing.png", "/.gitkeep"} {
		if w := get(path); w.Code != 404 {
			t.Fatalf("%s 应为 404，得到 %d", path, w.Code)
		}
	}
	if w := get("/api/nope"); w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("API 不应被缓存")
	}
}

func TestStaticNotBuilt(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	h, err := Handler(db, fakeStatus{}, config.Config{Location: time.UTC, Timezone: "UTC"}, fstest.MapFS{".gitkeep": {}})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("未构建前端时应为 503，得到 %d", w.Code)
	}
}
