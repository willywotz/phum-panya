package auth

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// DBLimiter is a Limiter whose failure state lives in the database, so it is
// shared across api replicas. It reproduces Throttle's sliding-window rule:
// a key is blocked once it has max or more failures within window.
type DBLimiter struct {
	db       *gorm.DB
	clk      clock.Clock
	max      int
	window   time.Duration
	fallback *Throttle
	logger   *slog.Logger
	errs     metric.Int64Counter
}

var _ Limiter = (*DBLimiter)(nil)

// NewDBLimiter returns a DB-backed Limiter that degrades to a per-replica
// in-process Throttle when the database is unreachable.
func NewDBLimiter(g *gorm.DB, clk clock.Clock, max int, window time.Duration, logger *slog.Logger) (*DBLimiter, error) {
	errs, err := otel.Meter("phum-panya/auth").Int64Counter("login_throttle_store_error_count")
	if err != nil {
		return nil, err
	}
	return &DBLimiter{
		db:       g,
		clk:      clk,
		max:      max,
		window:   window,
		fallback: NewThrottle(clk, max, window),
		logger:   logger,
		errs:     errs,
	}, nil
}

func (l *DBLimiter) storeError(op string, err error) {
	l.errs.Add(context.Background(), 1, metric.WithAttributes(attribute.String("op", op)))
	l.logger.Warn("login throttle store error", "op", op, "err", err)
}

// Allowed reports whether key has fewer than max failures within window.
// If the store is unreachable it degrades to the in-process fallback.
func (l *DBLimiter) Allowed(key string) bool {
	cutoff := l.clk.Now().Add(-l.window)
	var n int64
	res := l.db.Model(&model.LoginAttempt{}).
		Where("key = ? AND created_at > ?", key, cutoff).
		Count(&n)
	if res.Error != nil {
		l.storeError("allowed", res.Error)
		return l.fallback.Allowed(key)
	}
	return n < int64(l.max)
}

// Fail prunes key's expired rows and records a failure at the current time.
// If the store is unreachable it records the failure in the in-process fallback.
func (l *DBLimiter) Fail(key string) {
	now := l.clk.Now()
	cutoff := now.Add(-l.window)
	l.db.Where("key = ? AND created_at <= ?", key, cutoff).Delete(&model.LoginAttempt{})
	if res := l.db.Create(&model.LoginAttempt{Key: key, CreatedAt: now}); res.Error != nil {
		l.storeError("fail", res.Error)
		l.fallback.Fail(key)
	}
}

// Reset clears all recorded failures for key.
// If the store is unreachable it clears the in-process fallback instead.
func (l *DBLimiter) Reset(key string) {
	if res := l.db.Where("key = ?", key).Delete(&model.LoginAttempt{}); res.Error != nil {
		l.storeError("reset", res.Error)
		l.fallback.Reset(key)
	}
}
