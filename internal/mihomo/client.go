// Package mihomo exposes only the three read-only controller operations used by
// the collector. No generic controller proxy is available to the HTTP server.
package mihomo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/coder/websocket"
)

type Metadata struct {
	Type          string `json:"type"`
	Host          string `json:"host"`
	SniffHost     string `json:"sniffHost"`
	DestinationIP string `json:"destinationIP"`
}
type Connection struct {
	ID       string    `json:"id"`
	Metadata Metadata  `json:"metadata"`
	Upload   int64     `json:"upload"`
	Download int64     `json:"download"`
	Start    time.Time `json:"start"`
	Chains   []string  `json:"chains"`
}
type Snapshot struct {
	UploadTotal   int64        `json:"uploadTotal"`
	DownloadTotal int64        `json:"downloadTotal"`
	Connections   []Connection `json:"connections"`
}
type Proxy struct {
	Type string `json:"type"`
}

type Client struct {
	base   string
	secret func() (string, error)
	http   *http.Client
}
type readOnlyTransport struct{ base http.RoundTripper }

func (t readOnlyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Method != http.MethodGet {
		return nil, errors.New("Mihomo 控制器仅允许 GET")
	}
	return t.base.RoundTrip(r)
}
func New(base string, secret func() (string, error)) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &Client{base: strings.TrimRight(base, "/"), secret: secret, http: &http.Client{
		Transport:     readOnlyTransport{transport},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}
}
func (c *Client) headers() (http.Header, error) {
	s, err := c.secret()
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	if s != "" {
		h.Set("Authorization", "Bearer "+s)
	}
	return h, nil
}
func (c *Client) get(ctx context.Context, path string, dst any) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	h, err := c.headers()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return errors.New("Mihomo 地址无效")
	}
	req.Header = h
	res, err := c.http.Do(req)
	if err != nil {
		return errors.New("无法连接 Mihomo 控制器")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return statusError(res.StatusCode)
	}
	if err = json.NewDecoder(io.LimitReader(res.Body, 16<<20)).Decode(dst); err != nil {
		return errors.New("Mihomo 返回的数据格式无效")
	}
	return nil
}
func statusError(code int) error {
	if code == 401 || code == 403 {
		return errors.New("Mihomo 认证失败，请检查 secret_file 或 MIHOMO_SECRET")
	}
	return fmt.Errorf("Mihomo 返回 HTTP %d", code)
}
func (c *Client) Version(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"version"`
	}
	err := c.get(ctx, "/version", &v)
	return v.Version, err
}
func (c *Client) Proxies(ctx context.Context) (map[string]Proxy, error) {
	var v struct {
		Proxies map[string]Proxy `json:"proxies"`
	}
	err := c.get(ctx, "/proxies", &v)
	return v.Proxies, err
}
func (c *Client) Stream(ctx context.Context, interval time.Duration) (*Stream, error) {
	h, err := c.headers()
	if err != nil {
		return nil, err
	}
	dialCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	conn, res, err := websocket.Dial(dialCtx, fmt.Sprintf("%s/connections?interval=%d", c.base, interval.Milliseconds()), &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: h})
	if err != nil {
		if res != nil && res.StatusCode != http.StatusSwitchingProtocols {
			return nil, statusError(res.StatusCode)
		}
		return nil, errors.New("Mihomo WebSocket 连接失败")
	}
	conn.SetReadLimit(16 << 20)
	return &Stream{conn}, nil
}

type Stream struct{ conn *websocket.Conn }

func (s *Stream) Read(ctx context.Context) (Snapshot, error) {
	var snap Snapshot
	_, b, err := s.conn.Read(ctx)
	if err != nil {
		return snap, errors.New("连接快照流中断或超时")
	}
	if err = json.Unmarshal(b, &snap); err != nil {
		return snap, errors.New("连接快照格式无效")
	}
	return snap, nil
}
func (s *Stream) Close() { _ = s.conn.CloseNow() }
