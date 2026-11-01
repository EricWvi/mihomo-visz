package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	_ "time/tzdata"

	"gopkg.in/yaml.v3"
)

const (
	DefaultFlushInterval = 30 * time.Second
	DefaultRetentionDays = 7
)

type Config struct {
	Listen        string        `yaml:"listen"`
	FlushInterval time.Duration `yaml:"flush_interval"`
	RetentionDays int           `yaml:"retention_days"`
	DataDir       string        `yaml:"data_dir"`
	Timezone      string        `yaml:"timezone"`
	Mihomo        struct {
		URL        string `yaml:"url"`
		SecretFile string `yaml:"secret_file"`
		Interval   string `yaml:"interval"`
		StaleAfter string `yaml:"stale_after"`
	} `yaml:"mihomo"`
	Location   *time.Location `yaml:"-"`
	Interval   time.Duration  `yaml:"-"`
	StaleAfter time.Duration  `yaml:"-"`
}

func Load(path string) (Config, error) {
	var c Config
	c.Listen = "127.0.0.1:36750"
	c.FlushInterval = DefaultFlushInterval
	c.RetentionDays = DefaultRetentionDays
	c.DataDir = "./.data"
	c.Timezone = "Asia/Shanghai"
	c.Mihomo.URL = "http://127.0.0.1:9090"
	c.Mihomo.Interval = "1s"
	c.Mihomo.StaleAfter = "10s"
	f, err := os.Open(path)
	if err != nil {
		return c, fmt.Errorf("读取配置: %w", err)
	}
	defer f.Close()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	if err = dec.Decode(&c); err != nil {
		return c, fmt.Errorf("解析配置: %w", err)
	}
	var extra any
	if err = dec.Decode(&extra); err != io.EOF {
		return c, errors.New("配置仅允许一个 YAML 文档")
	}
	if c.Listen == "" || c.DataDir == "" {
		return c, errors.New("listen 和 data_dir 不能为空")
	}
	u, err := url.Parse(c.Mihomo.URL)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return c, errors.New("mihomo.url 必须是无认证参数的 HTTP(S) 地址")
	}
	c.Mihomo.URL = strings.TrimRight(c.Mihomo.URL, "/")
	if c.Location, err = time.LoadLocation(c.Timezone); err != nil {
		return c, errors.New("timezone 无效")
	}
	if c.Interval, err = time.ParseDuration(c.Mihomo.Interval); err != nil || c.Interval < 100*time.Millisecond || c.Interval > time.Minute {
		return c, errors.New("mihomo.interval 应在 100ms 到 1m 之间")
	}
	if c.StaleAfter, err = time.ParseDuration(c.Mihomo.StaleAfter); err != nil || c.StaleAfter < c.Interval*3 || c.StaleAfter > 10*time.Minute {
		return c, errors.New("mihomo.stale_after 应至少为 interval 的三倍，且不超过 10m")
	}
	if c.FlushInterval < time.Second || c.FlushInterval > 5*time.Minute {
		return c, errors.New("flush_interval 应在 1s 到 5m 之间")
	}
	if c.RetentionDays < 1 || c.RetentionDays > 3650 {
		return c, errors.New("retention_days 应在 1 到 3650 之间")
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, err
	}
	resolve := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(base, p)
	}
	c.DataDir = resolve(c.DataDir)
	c.Mihomo.SecretFile = resolve(c.Mihomo.SecretFile)
	return c, nil
}

// Secret is read only by the outbound client. It is never part of an API response.
func (c Config) Secret() (string, error) {
	if secret := strings.TrimSpace(os.Getenv("MIHOMO_SECRET")); secret != "" {
		return secret, nil
	}
	if c.Mihomo.SecretFile == "" {
		return "", nil
	}
	b, err := os.ReadFile(c.Mihomo.SecretFile)
	if err != nil {
		return "", errors.New("无法读取 Mihomo secret_file，请检查文件路径和权限")
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", errors.New("Mihomo secret_file 为空")
	}
	return s, nil
}
