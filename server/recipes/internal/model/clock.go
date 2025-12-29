package model

import "time"

// SystemClock implements Clock using the system time.
type SystemClock struct{}

// Now returns the current system time.
func (SystemClock) Now() time.Time {
	return time.Now()
}
