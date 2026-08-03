package httpx_test

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"phum-panya/internal/config"
	"phum-panya/internal/httpx"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	return addr
}

func waitUp(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server never came up on %s", addr)
}

// ServeContext serves until the context is cancelled, then shuts down and
// returns nil — this is what lets the service supervisor stop the server.
func TestServeContextShutsDownOnCancel(t *testing.T) {
	addr := freeAddr(t)
	cfg := config.Config{HTTPAddr: addr, DevMode: true}
	mux := http.NewServeMux()
	mux.HandleFunc("/ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- httpx.ServeContext(ctx, cfg, mux) }()

	waitUp(t, addr)
	res, err := http.Get("http://" + addr + "/ping")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ServeContext returned %v, want nil", err)
		}
	case <-time.After(11 * time.Second):
		t.Fatal("ServeContext did not return after cancel")
	}
}
