package message

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestProductionRegressionFixtureIsSanitized(t *testing.T) {
	data, err := os.ReadFile("testdata/production/regressions.json")
	require.NoError(t, err)
	var fixture struct {
		Cases []struct {
			ID       string `json:"id"`
			Observed string `json:"observed"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	require.Len(t, fixture.Cases, 6)
	for _, item := range fixture.Cases {
		require.NotEmpty(t, item.ID)
		require.NotEmpty(t, item.Observed)
		require.NotContains(t, item.Observed, "https://")
		require.NotContains(t, item.Observed, "token")
	}
}
