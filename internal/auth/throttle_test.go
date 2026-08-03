package auth_test

import (
	"testing"
	"time"

	"phum-panya/internal/auth"
	"phum-panya/internal/clock"
)

func TestThrottleAllowedThenBlockedThenWindowSlides(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	fake := &clock.Fake{T: now}
	th := auth.NewThrottle(fake, 3, time.Minute)

	const key = "a@x|1.2.3.4"
	for i := 0; i < 3; i++ {
		if !th.Allowed(key) {
			t.Fatalf("Allowed(%d) = false, want true", i)
		}
		th.Fail(key)
	}

	if th.Allowed(key) {
		t.Fatal("Allowed after max failures = true, want false")
	}

	fake.T = fake.T.Add(time.Minute + time.Second)
	if !th.Allowed(key) {
		t.Fatal("Allowed after window slid = false, want true")
	}
}
