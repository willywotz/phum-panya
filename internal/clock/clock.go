package clock

import "time"

// Clock provides access to the current time.
type Clock interface {
	Now() time.Time
}

// Real returns the actual current time in UTC.
type Real struct{}

// Now returns the current time in UTC.
func (Real) Now() time.Time {
	return time.Now().UTC()
}

// Fake returns a fixed time for deterministic testing.
type Fake struct {
	T time.Time
}

// Now returns the fixed time.
func (f Fake) Now() time.Time {
	return f.T
}
