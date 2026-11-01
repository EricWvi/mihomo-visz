package server

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mihomo-visz/internal/collector"
	"mihomo-visz/internal/config"
	"mihomo-visz/internal/store"
)

type statusReader interface{ Status() collector.Status }

func Handler(db *store.Store, source statusReader, cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, struct {
			collector.Status
			Timezone string `json:"timezone"`
			Today    string `json:"today"`
			Now      int64  `json:"now"`
		}{source.Status(), cfg.Timezone, time.Now().In(cfg.Location).Format(time.DateOnly), time.Now().UnixMilli()})
	})
	mux.HandleFunc("GET /api/traffic", func(w http.ResponseWriter, r *http.Request) {
		f, ok := parseFilter(w, r, cfg)
		if !ok {
			return
		}
		result, err := db.Traffic(r.Context(), f)
		if err != nil {
			slog.Error("查询流量失败", "error", err)
			writeError(w, 500, "无法读取流量数据")
			return
		}
		hours := []int64{}
		for h := f.From; h.Before(f.To); h = h.Add(time.Hour) {
			hours = append(hours, h.Unix())
		}
		writeJSON(w, 200, struct {
			store.Traffic
			Hours    []int64 `json:"hours"`
			Date     string  `json:"date"`
			Timezone string  `json:"timezone"`
		}{result, hours, f.From.Format(time.DateOnly), cfg.Timezone})
	})
	mux.HandleFunc("GET /api/export", func(w http.ResponseWriter, r *http.Request) {
		f, ok := parseFilter(w, r, cfg)
		if !ok {
			return
		}
		result, err := db.Traffic(r.Context(), f)
		if err != nil {
			writeError(w, 500, "无法导出流量数据")
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="traffic-`+f.From.Format(time.DateOnly)+`.csv"`)
		_, _ = w.Write([]byte("\xef\xbb\xbf"))
		out := csv.NewWriter(w)
		_ = out.Write([]string{"小时开始", "节点", "域名", "上传字节", "下载字节", "总字节"})
		// Prevent spreadsheet software from treating observed host/node names as formulas.
		safe := func(s string) string {
			if strings.HasPrefix(strings.TrimSpace(s), "=") || strings.HasPrefix(strings.TrimSpace(s), "+") || strings.HasPrefix(strings.TrimSpace(s), "-") || strings.HasPrefix(strings.TrimSpace(s), "@") {
				return "'" + s
			}
			return s
		}
		for _, row := range result.Rows {
			_ = out.Write([]string{time.Unix(row.Hour, 0).In(cfg.Location).Format(time.RFC3339), safe(row.Node), safe(row.Domain), strconv.FormatInt(row.Upload, 10), strconv.FormatInt(row.Download, 10), strconv.FormatInt(row.Upload+row.Download, 10)})
		}
		out.Flush()
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) { writeError(w, 404, "接口不存在") })
	files := http.FileServer(http.Dir(cfg.FrontendDir))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			if _, err := os.Stat(filepath.Join(cfg.FrontendDir, "index.html")); err != nil {
				http.Error(w, "前端尚未构建，请执行 task build，或通过 Vite 开发服务访问。", http.StatusServiceUnavailable)
				return
			}
		}
		files.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		mux.ServeHTTP(w, r.WithContext(ctx))
	})
}
func parseFilter(w http.ResponseWriter, r *http.Request, cfg config.Config) (store.Filter, bool) {
	var f store.Filter
	q := r.URL.Query()
	day := q.Get("date")
	if day == "" {
		day = time.Now().In(cfg.Location).Format(time.DateOnly)
	}
	from, err := time.ParseInLocation(time.DateOnly, day, cfg.Location)
	if err != nil {
		writeError(w, 400, "date 应为 YYYY-MM-DD")
		return f, false
	}
	f.From = from
	f.To = from.AddDate(0, 0, 1)
	f.Node = q.Get("node")
	f.Search = strings.TrimSpace(q.Get("domain"))
	f.Group = q.Get("group")
	if f.Group == "" {
		f.Group = "domain"
	}
	if f.Group != "domain" && f.Group != "host" {
		writeError(w, 400, "group 应为 domain 或 host")
		return f, false
	}
	if len(f.Search) > 255 || len(f.Node) > 1024 {
		writeError(w, 400, "筛选条件过长")
		return f, false
	}
	return f, true
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
