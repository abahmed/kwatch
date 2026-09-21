package clock

import (
	"testing"
	"time"
)

func TestClockAdaptersUseInjectedTime(t *testing.T) {
	want := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	got := Func(func() time.Time { return want }).Now()
	if !got.Equal(want) {
		t.Fatalf("clock time = %v, want %v", got, want)
	}
	if (RealClock{}).Now().IsZero() {
		t.Fatal("real clock returned zero time")
	}
}

func TestClockRequirementsRejectMissingDependencies(t *testing.T) {
	assertPanics(t, func() { Require(nil) })
	assertPanics(t, func() { RequireFunc(nil) })
}

func assertPanics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("function did not panic")
		}
	}()
	fn()
}
