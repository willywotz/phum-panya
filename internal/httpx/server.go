package httpx

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/crypto/acme/autocert"

	"phum-panya/internal/config"
)

// Serve runs the HTTP server for cfg until SIGINT/SIGTERM, then shuts down
// gracefully. It is the foreground entry point; ServeContext is the same
// server driven by an explicit context (used by the service supervisor).
func Serve(cfg config.Config, h http.Handler) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return ServeContext(ctx, cfg, h)
}

// ServeContext runs the HTTP server for cfg, handling h. In dev mode or when
// no domain is configured it serves plain HTTP on cfg.HTTPAddr. Otherwise it
// serves TLS on :443 via autocert, with a second server on :80 for the ACME
// HTTP-01 challenge and HTTPS redirect. It blocks until a listen error occurs
// or ctx is cancelled, then shuts both servers down gracefully.
func ServeContext(ctx context.Context, cfg config.Config, h http.Handler) error {
	if cfg.DevMode || cfg.Domain == "" {
		return serveHTTP(ctx, &http.Server{Addr: cfg.HTTPAddr, Handler: h})
	}
	return serveTLS(ctx, cfg, h)
}

func serveHTTP(ctx context.Context, srv *http.Server) error {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(shutCtx)
	}
}

func serveTLS(ctx context.Context, cfg config.Config, h http.Handler) error {
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		HostPolicy: autocert.HostWhitelist(cfg.Domain),
		Cache:      autocert.DirCache("data/certs"),
	}
	main := &http.Server{
		Addr:      ":443",
		Handler:   h,
		TLSConfig: &tls.Config{GetCertificate: manager.GetCertificate},
	}
	redirect := &http.Server{Addr: ":80", Handler: manager.HTTPHandler(nil)}

	errCh := make(chan error, 2)
	go func() {
		if err := main.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()
	go func() {
		if err := redirect.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		mainErr := main.Shutdown(shutCtx)
		redirectErr := redirect.Shutdown(shutCtx)
		if mainErr != nil {
			return mainErr
		}
		return redirectErr
	}
}
