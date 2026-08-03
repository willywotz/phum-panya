package clock

import (
	"testing"
	"time"
)

func TestFakeNow(t *testing.T) {
	expected := time.Date(2026, 8, 3, 12, 30, 45, 0, time.UTC)
	fake := Fake{T: expected}
	got := fake.Now()

	if !got.Equal(expected) {
		t.Errorf("Fake.Now() = %v, want %v", got, expected)
	}
}

func TestRealNow(t *testing.T) {
	real := Real{}
	before := time.Now().UTC()
	got := real.Now()
	after := time.Now().UTC()

	if got.Before(before) || got.After(after) {
		t.Errorf("Real.Now() = %v, expected time between %v and %v", got, before, after)
	}
}

func TestClockInterface(t *testing.T) {
	var _ Clock = Real{}
	var _ Clock = Fake{T: time.Now()}
}
