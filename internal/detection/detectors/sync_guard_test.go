package detectors

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// routeReadersWithoutSyncCheck are the detectors that read RoutesTo
// without a sync check, each with why that is safe: a route that is
// missing from the model can only make a finding quieter.
var routeReadersWithoutSyncCheck = map[string]string{
	"loadbalancer_wait.go": "a route only raises a pending load " +
		"balancer from Info to Warning",
	"deletion_leftover.go": "lists relation types that make an object " +
		"in use, and never concludes that an object is missing",
}

// TestDetectorsReadingEndpointSlicesCheckTheyAreSynced fails when a
// detector reads the EndpointSlices behind a Service (the Backs
// relation) without asking whether that kind is synced: a Service whose
// slices are not watched would look as if it had no backends, which is
// a finding about the cluster that kwatch cannot support.
func TestDetectorsReadingEndpointSlicesCheckTheyAreSynced(t *testing.T) {
	for name, source := range detectorSources(t) {
		if strings.Contains(source, "inventory.Backs") {
			assert.True(t, strings.Contains(source,
				"Synced(kube.KindEndpointSlice)"),
				"%s reads EndpointSlices without checking they are synced",
				name)
		}
	}
}

func TestDetectorsReadingRoutesCheckSyncOrSayWhyNot(t *testing.T) {
	for name, source := range detectorSources(t) {
		if !strings.Contains(source, "inventory.RoutesTo") {
			continue
		}
		if _, excused := routeReadersWithoutSyncCheck[name]; excused {
			continue
		}
		assert.True(t, strings.Contains(source, ".Synced("),
			"%s reads routes without checking its sources are synced",
			name)
	}
}

// detectorSources returns the text of every non-test Go file of the
// package, by file name.
func detectorSources(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	require.NoError(t, err)
	out := map[string]string{}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		out[file] = string(data)
	}
	return out
}
