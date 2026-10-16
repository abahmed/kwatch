//go:build licensetest

package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionJSONNamesTheProEditionOfATestBuild(t *testing.T) {
	t.Setenv("KWATCH_EDITION_OVERRIDE", "pro")
	var out, errOut bytes.Buffer
	code := runCommand([]string{"version", "--json"}, &out, &errOut,
		func() int { return 99 })
	if code != 0 || !strings.Contains(out.String(), `"edition":"pro"`) {
		t.Fatalf("code=%d out=%q", code, out.String())
	}
}
