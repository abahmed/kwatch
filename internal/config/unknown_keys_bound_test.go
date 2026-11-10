package config

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

// unknownKeysFile is a config with n unknown keys under app.
func unknownKeysFile(n int) []byte {
	var b strings.Builder
	b.WriteString("app:\n")
	for i := range n {
		fmt.Fprintf(&b, "  bogus%d: x\n", i)
	}
	return []byte(b.String())
}

// Many unknown keys are found in one walk of the file and reported up
// to a bound, the rest counted in one entry. The walk grows linearly:
// ten times the keys costs about ten times as much, where a search
// per key cost about a hundred times. Checked as growth, not wall
// time, so slow or busy machines pass.
func TestUnknownKeysAreBoundedAndFast(t *testing.T) {
	timed := func(n int) time.Duration {
		file := unknownKeysFile(n)
		runs := make([]time.Duration, 3)
		for i := range runs {
			start := time.Now()
			unknownConfigKeys(file)
			runs[i] = time.Since(start)
		}
		slices.Sort(runs)
		return runs[1]
	}
	small, big := timed(150), timed(1500)
	if big > 30*small+5*time.Millisecond {
		t.Errorf("1500 keys took %v, 150 keys %v: not linear", big, small)
	}
	keys := unknownConfigKeys(unknownKeysFile(1500))
	if len(keys) != maxUnknownKeys+1 {
		t.Fatalf("got %d entries, want %d", len(keys), maxUnknownKeys+1)
	}
	if keys[0] != "line 2: app.bogus0" {
		t.Errorf("first entry %q", keys[0])
	}
	warn := unknownKeyWarnings(&Config{unknownKeys: keys})
	if last := warn[len(warn)-1]; !strings.HasPrefix(last,
		"1450 more config keys") {
		t.Errorf("summary warning %q", last)
	}
}
