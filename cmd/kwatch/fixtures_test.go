package main

import "time"

// testNow is the clock the commands under test read.
func testNow() time.Time {
	return time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
}
