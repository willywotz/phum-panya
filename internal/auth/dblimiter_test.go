package auth_test

import (
	"path/filepath"
	"testing"
	"time"

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

func TestDBLimiterAllowedThenBlockedThenWindowSlides(t *testing.T) {
	fake := &clock.Fake{T: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)}
	l := auth.NewDBLimiter(newLimiterDB(t), fake, 3, time.Minute)
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
	l := auth.NewDBLimiter(newLimiterDB(t), fake, 3, time.Minute)
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
	a := auth.NewDBLimiter(g, fake, 3, time.Minute)
	b := auth.NewDBLimiter(g, fake, 3, time.Minute)
	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		a.Fail(key)
	}
	if b.Allowed(key) {
		t.Fatal("instance B allows after instance A hit max; throttle state is not shared")
	}
}
