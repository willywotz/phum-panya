package httpx_test

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"phum-panya/internal/config"
	"phum-panya/internal/httpx"
)

// TestServeContextBehindProxyServesPlainHTTP proves that with a domain set but
// BehindProxy on, the server serves plain HTTP on HTTPAddr and never tries ACME.
func TestServeContextBehindProxyServesPlainHTTP(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	cfg := config.Config{HTTPAddr: addr, Domain: "example.org", BehindProxy: true}
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "ok")
	})
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- httpx.ServeContext(ctx, cfg, h) }()

	var resp *http.Response
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	cancel()
	if err := <-errCh; err != nil {
		t.Errorf("serve returned %v", err)
	}
}
