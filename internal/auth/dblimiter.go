package auth

import (
	"time"

	"gorm.io/gorm"

	"phum-panya/internal/clock"
	"phum-panya/internal/model"
)

// DBLimiter is a Limiter whose failure state lives in the database, so it is
// shared across api replicas. It reproduces Throttle's sliding-window rule:
// a key is blocked once it has max or more failures within window.
type DBLimiter struct {
	db     *gorm.DB
	clk    clock.Clock
	max    int
	window time.Duration
}

var _ Limiter = (*DBLimiter)(nil)

// NewDBLimiter returns a DB-backed Limiter.
func NewDBLimiter(g *gorm.DB, clk clock.Clock, max int, window time.Duration) *DBLimiter {
	return &DBLimiter{db: g, clk: clk, max: max, window: window}
}

// Allowed reports whether key has fewer than max failures within window.
func (l *DBLimiter) Allowed(key string) bool {
	cutoff := l.clk.Now().Add(-l.window)
	var n int64
	l.db.Model(&model.LoginAttempt{}).
		Where("key = ? AND created_at > ?", key, cutoff).
		Count(&n)
	return n < int64(l.max)
}

// Fail prunes key's expired rows and records a failure at the current time.
func (l *DBLimiter) Fail(key string) {
	now := l.clk.Now()
	cutoff := now.Add(-l.window)
	l.db.Where("key = ? AND created_at <= ?", key, cutoff).Delete(&model.LoginAttempt{})
	l.db.Create(&model.LoginAttempt{Key: key, CreatedAt: now})
}

// Reset clears all recorded failures for key.
func (l *DBLimiter) Reset(key string) {
	l.db.Where("key = ?", key).Delete(&model.LoginAttempt{})
}
