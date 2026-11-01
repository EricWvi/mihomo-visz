package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigRelativePathsAndSecretFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(filepath.Join(dir, "secret"), []byte("  fake-test-only-secret\n"), 0600)
	os.WriteFile(path, []byte("listen: 127.0.0.1:12345\ndata_dir: data\nmihomo:\n  secret_file: secret\n"), 0600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DataDir != filepath.Join(dir, "data") || cfg.Mihomo.SecretFile != filepath.Join(dir, "secret") {
		t.Fatal("相对路径没有按配置目录解析")
	}
	if cfg.FlushInterval != DefaultFlushInterval || cfg.RetentionDays != DefaultRetentionDays {
		t.Fatal("默认落盘周期或保留期错误")
	}
	t.Setenv("MIHOMO_SECRET", "")
	secret, err := cfg.Secret()
	if err != nil || secret != "fake-test-only-secret" {
		t.Fatal("文件密钥读取失败")
	}
	t.Setenv("MIHOMO_SECRET", "env-test-value")
	secret, err = cfg.Secret()
	if err != nil || secret != "env-test-value" {
		t.Fatal("环境变量优先级错误")
	}
}
func TestConfigRejectsUnsafeAndInvalidSettings(t *testing.T) {
	for _, raw := range []string{
		"mihomo:\n  url: http://user:password@localhost:9090\n",
		"mihomo:\n  url: http://localhost:9090?token=private\n",
		"mihomo:\n  interval: 0s\n",
		"mihomo:\n  stale_after: 1s\n",
		"timezone: nowhere\n",
		"flush_interval: 0s\n",
		"flush_interval: 10m\n",
		"retention_days: 0\n",
		"retention_days: -1\n",
		"unknown_setting: true\n",
		"listen: localhost:1234\n---\nlisten: localhost:4567\n",
	} {
		p := filepath.Join(t.TempDir(), "config.yaml")
		os.WriteFile(p, []byte(raw), 0600)
		if _, err := Load(p); err == nil {
			t.Errorf("无效配置被接受: %s", raw)
		}
	}
}

func TestStoragePolicyCanBeConfigured(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	os.WriteFile(path, []byte("flush_interval: 10s\nretention_days: 14\n"), 0600)
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.FlushInterval.String() != "10s" || cfg.RetentionDays != 14 {
		t.Fatal("落盘或保留策略未加载")
	}
}
