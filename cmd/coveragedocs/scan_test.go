package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeTree writes files under a temporary repository root.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, body := range files {
		path := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	}
	return root
}

// A detector that only compares a reason does not raise it.
func TestScanListsOnlyDetectorsThatRaiseTheReason(t *testing.T) {
	root := writeTree(t, map[string]string{
		"internal/detection/reasons/reasons.go": `package reasons
const Boom = "Boom"
`,
		"internal/detection/detectors/raiser.go": `package detectors
func raise() any {
	return Finding{Reason: reasons.Boom, Severity: detection.Critical}
}
`,
		"internal/detection/detectors/reader.go": `package detectors
func read(r string) bool {
	switch r {
	case reasons.Boom:
		return true
	}
	return r == reasons.Boom || r != reasons.Boom
}
`,
	})

	usage, err := scanDetectors(root)

	require.NoError(t, err)
	assert.Equal(t, []string{"raiser"}, usage.detectors([]string{"Boom"}))
}
