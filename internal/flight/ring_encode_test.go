package flight

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

// freshEncoding is how a batch was encoded before writers were pooled:
// a new gzip writer for every batch.
func freshEncoding(t *testing.T, entries []replay.Entry) []byte {
	t.Helper()
	var raw bytes.Buffer
	zip := gzip.NewWriter(&raw)
	encoder := json.NewEncoder(zip)
	for _, entry := range entries {
		require.NoError(t, encoder.Encode(entry))
	}
	require.NoError(t, zip.Close())
	return raw.Bytes()
}

func encodeEntries(n int, phase string) []replay.Entry {
	out := make([]replay.Entry, 0, n)
	for i := 0; i < n; i++ {
		at := base.Add(time.Duration(i) * time.Second)
		out = append(out, replay.Entry{At: at,
			Observation: inventory.Observation{
				Kind: inventory.Observed, Source: "k8s", At: at,
				Entity: inventory.CoreID("pod", "shop",
					fmt.Sprintf("api-%d", i)),
				Attributes: map[string]inventory.Value{
					"phase": inventory.Text(phase)},
			}})
	}
	return out
}

func TestEncodeBatchMatchesAFreshWriter(t *testing.T) {
	inputs := [][]replay.Entry{
		encodeEntries(500, "Running"), encodeEntries(3, "Pending"),
		encodeEntries(500, "Running"), encodeEntries(1, "Failed"),
	}
	for round := 0; round < 3; round++ {
		for i, entries := range inputs {
			got, err := encodeBatch(entries)
			require.NoError(t, err)
			assert.Equal(t, freshEncoding(t, entries), got,
				"round %d input %d", round, i)
		}
	}
}

func TestEncodeBatchKeepsEarlierOutputs(t *testing.T) {
	first, err := encodeBatch(encodeEntries(50, "Running"))
	require.NoError(t, err)
	kept := bytes.Clone(first)
	_, err = encodeBatch(encodeEntries(80, "Pending"))
	require.NoError(t, err)
	assert.Equal(t, kept, first)
	decoded, err := decodeBatch(Batch{Z: first, N: 50})
	require.NoError(t, err)
	assert.Len(t, decoded, 50)
}
