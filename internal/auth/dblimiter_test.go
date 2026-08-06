package auth_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	metricsdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"gorm.io/gorm"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
	"phum-panya/internal/db"
)

func newLimiterDB(t *testing.T) *gorm.DB {
	t.Helper()
	g, err := db.Open(filepath.Join(t.TempDir(), "throttle.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	if err := db.AutoMigrate(g); err != nil {
		t.Fatalf("AutoMigrate: %v", err)
	}
	return g
}

func newTestLimiter(t *testing.T, g *gorm.DB, clk clock.Clock, max int, window time.Duration) *auth.DBLimiter {
	t.Helper()
	l, err := auth.NewDBLimiter(g, clk, max, window, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewDBLimiter: %v", err)
	}
	return l
}

func TestDBLimiterAllowedThenBlockedThenWindowSlides(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l := newTestLimiter(t, newLimiterDB(t), fake, 3, time.Minute)
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		if !l.Allowed(key) {
			t.Fatalf("Allowed(%d) = false, want true", i)
		}
		l.Fail(key)
	}
	if l.Allowed(key) {
		t.Fatal("Allowed after max failures = true, want false")
	}
	fake.T = fake.T.Add(time.Minute + time.Second)
	if !l.Allowed(key) {
		t.Fatal("Allowed after window slid = false, want true")
	}
}

func TestDBLimiterResetClearsFailures(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l := newTestLimiter(t, newLimiterDB(t), fake, 3, time.Minute)
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		l.Fail(key)
	}
	l.Reset(key)
	if !l.Allowed(key) {
		t.Fatal("Allowed after Reset = false, want true")
	}
}

func TestDBLimiterSharedAcrossInstances(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	g := newLimiterDB(t)
	a := newTestLimiter(t, g, fake, 3, time.Minute)
	b := newTestLimiter(t, g, fake, 3, time.Minute)
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		a.Fail(key)
	}
	if b.Allowed(key) {
		t.Fatal("instance B allows after instance A hit max; throttle state is not shared")
	}
}

func closedDB(t *testing.T) *gorm.DB {
	t.Helper()
	g := newLimiterDB(t)
	sqlDB, err := g.DB()
	if err != nil {
		t.Fatalf("g.DB: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return g
}

func TestDBLimiterDegradesToFallbackWhenStoreFails(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l, err := auth.NewDBLimiter(closedDB(t), fake, 3, time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewDBLimiter: %v", err)
	}
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		if !l.Allowed(key) {
			t.Fatalf("Allowed(%d) = false, want true (fallback under threshold)", i)
		}
		l.Fail(key)
	}
	if l.Allowed(key) {
		t.Fatal("Allowed after max fallback failures = true, want false")
	}
}

func TestDBLimiterStoreErrorLogsWarn(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	l, err := auth.NewDBLimiter(closedDB(t), fake, 3, time.Minute, logger)
	if err != nil {
		t.Fatalf("NewDBLimiter: %v", err)
	}
	l.Allowed("k")
	out := buf.String()
	if !strings.Contains(out, "login throttle store error") || !strings.Contains(out, "op=allowed") {
		t.Fatalf("missing WARN store-error log, got: %q", out)
	}
}

func TestDBLimiterStoreErrorIncrementsCounter(t *testing.T) {
	reader := metricsdk.NewManualReader()
	otel.SetMeterProvider(metricsdk.NewMeterProvider(metricsdk.WithReader(reader)))
	t.Cleanup(func() { otel.SetMeterProvider(otel.GetMeterProvider()) })

	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l, err := auth.NewDBLimiter(closedDB(t), fake, 3, time.Minute,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewDBLimiter: %v", err)
	}
	l.Allowed("k")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if !hasCounter(rm, "login_throttle_store_error_count") {
		t.Fatal("login_throttle_store_error_count not recorded")
	}
}

func hasCounter(rm metricdata.ResourceMetrics, name string) bool {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return true
			}
		}
	}
	return false
}
