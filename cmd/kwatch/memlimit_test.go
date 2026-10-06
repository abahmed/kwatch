package main

import (
	"math"
	"runtime/debug"
	"testing"
)

func TestApplyMemoryLimit(t *testing.T) {
	previous := debug.SetMemoryLimit(-1)
	t.Cleanup(func() { debug.SetMemoryLimit(previous) })
	env := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}

	got, err := applyMemoryLimit(env(map[string]string{
		memoryLimitEnv: "536870912", // 512Mi
	}))
	if err != nil {
		t.Fatal(err)
	}
	if want := int64(536870912 / 100 * 90); got != want {
		t.Fatalf("limit = %d, want %d", got, want)
	}
	if debug.SetMemoryLimit(-1) != got {
		t.Fatal("the runtime limit was not set")
	}

	debug.SetMemoryLimit(math.MaxInt64)
	for name, values := range map[string]map[string]string{
		"explicit GOMEMLIMIT wins": {
			"GOMEMLIMIT": "100MiB", memoryLimitEnv: "536870912",
		},
		"no limit": {},
	} {
		if got, err := applyMemoryLimit(env(values)); got != 0 ||
			err != nil {
			t.Fatalf("%s: limit must be left alone", name)
		}
	}
	if debug.SetMemoryLimit(-1) != math.MaxInt64 {
		t.Fatal("the runtime limit must not change")
	}
}

func TestApplyMemoryLimitRejectsInvalidValue(t *testing.T) {
	for _, raw := range []string{"512Mi", "0", "-5"} {
		_, err := applyMemoryLimit(func(key string) string {
			if key == memoryLimitEnv {
				return raw
			}
			return ""
		})
		if err == nil {
			t.Fatalf("%q must be an error", raw)
		}
	}
}
