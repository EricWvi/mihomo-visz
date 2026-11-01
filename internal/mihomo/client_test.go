package mihomo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestReadOnlyClientAndSecretHeader(t *testing.T) {
	const secret = "test-secret-never-return-in-errors"
	requests := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.Path
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Error("缺少认证头")
		}
		switch r.URL.Path {
		case "/version":
			w.Write([]byte(`{"version":"test"}`))
		case "/proxies":
			w.Write([]byte(`{"proxies":{"节点":{"type":"Trojan"}}}`))
		case "/connections":
			if r.URL.Query().Get("interval") != "1000" {
				t.Error("采样参数错误")
			}
			c, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer c.CloseNow()
			if err = c.Write(r.Context(), websocket.MessageText, []byte(`{"uploadTotal":1,"downloadTotal":2,"connections":[]}`)); err != nil {
				return
			}
			<-r.Context().Done()
		default:
			t.Error("请求了额外接口", r.URL.Path)
		}
	}))
	defer srv.Close()
	client := New(srv.URL, func() (string, error) { return secret, nil })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if v, err := client.Version(ctx); err != nil || v != "test" {
		t.Fatalf("version %s %v", v, err)
	}
	if ps, err := client.Proxies(ctx); err != nil || ps["节点"].Type != "Trojan" {
		t.Fatalf("proxies %v %v", ps, err)
	}
	stream, err := client.Stream(ctx, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	snap, err := stream.Read(ctx)
	stream.Close()
	if err != nil || snap.DownloadTotal != 2 {
		t.Fatalf("stream %+v %v", snap, err)
	}
	for range 3 {
		if req := <-requests; !strings.HasPrefix(req, "GET ") {
			t.Fatal(req)
		}
	}
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+"/connections", nil)
	if _, err = client.http.Do(req); err == nil {
		t.Fatal("写操作未被 transport 拦截")
	}
	select {
	case req := <-requests:
		t.Fatal("写操作到达了服务端", req)
	default:
	}
}
func TestControllerErrorsNeverEchoBodyOrSecret(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		w.Write([]byte("server echoed sensitive data"))
	}))
	defer srv.Close()
	client := New(srv.URL, func() (string, error) { return "private-test-value", nil })
	_, err := client.Version(context.Background())
	if err == nil || strings.Contains(err.Error(), "private-test-value") || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("不安全错误: %v", err)
	}
}
func TestClientDoesNotFollowRedirects(t *testing.T) {
	redirected := false
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { redirected = true }))
	defer target.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer srv.Close()
	_, err := New(srv.URL, func() (string, error) { return "", nil }).Version(context.Background())
	if err == nil || redirected {
		t.Fatalf("重定向不应被跟随: %v %v", err, redirected)
	}
}
func TestSecretFileFailureIsPropagated(t *testing.T) {
	client := New("http://127.0.0.1:1", func() (string, error) { return "", errors.New("密钥不可用") })
	if _, err := client.Version(context.Background()); err == nil || err.Error() != "密钥不可用" {
		t.Fatal(err)
	}
}
