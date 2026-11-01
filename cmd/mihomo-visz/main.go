package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mihomovisz "mihomo-visz"
	"mihomo-visz/internal/collector"
	"mihomo-visz/internal/config"
	"mihomo-visz/internal/mihomo"
	"mihomo-visz/internal/server"
	"mihomo-visz/internal/store"
)

// 构建时通过 -ldflags "-X main.version=... -X main.commit=..." 注入。
var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("启动失败", "error", err)
		os.Exit(1)
	}
}
func run() error {
	path := flag.String("config", "config.yaml", "YAML 配置文件路径")
	showVersion := flag.Bool("version", false, "打印版本后退出")
	flag.Parse()
	if *showVersion {
		fmt.Printf("mihomo-visz %s (%s)\n", version, commit)
		return nil
	}
	cfg, err := config.Load(*path)
	if err != nil {
		return err
	}
	db, err := store.Open(cfg.DataDir)
	if err != nil {
		return err
	}
	defer db.Close()
	client := mihomo.New(cfg.Mihomo.URL, cfg.Secret)
	source := collector.New(client, db, cfg)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	done := make(chan struct{})
	var sourceErr error
	go func() { defer close(done); sourceErr = source.Run(ctx) }()
	handler, err := server.Handler(db, source, cfg, mihomovisz.Frontend())
	if err != nil {
		return err
	}
	srv := http.Server{Addr: cfg.Listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	listenErr := make(chan error, 1)
	go func() { listenErr <- srv.ListenAndServe() }()
	slog.Info("Mihomo Visz 已启动", "listen", cfg.Listen, "timezone", cfg.Timezone, "version", version)
	select {
	case <-ctx.Done():
	case <-done:
		err = sourceErr
	case err = <-listenErr:
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
	}
	cancel()
	shutdownCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	if e := srv.Shutdown(shutdownCtx); e != nil && err == nil {
		err = e
	}
	<-done
	if err == nil {
		err = sourceErr
	}
	return err
}
