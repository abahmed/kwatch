package persistence

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/fake"

	kwcontext "github.com/abahmed/kwatch/internal/graphcontext"
)

func TestProductionRegressionFixtureCompactsOversizedChange(t *testing.T) {
	data, err := os.ReadFile("testdata/production/regressions.json")
	require.NoError(t, err)
	var fixture struct {
		Cases []struct {
			ID          string `json:"id"`
			DetailBytes int    `json:"detailBytes"`
		} `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	var detailBytes int
	for _, item := range fixture.Cases {
		if item.ID == "oversized-change-history" {
			detailBytes = item.DetailBytes
		}
	}
	require.Greater(t, detailBytes, maxChangeDetailBytes)
	manager := newTestManager(fake.NewSimpleClientset(), "kwatch")
	err = manager.SaveChangeHistory(context.Background(), []kwcontext.Change{{
		Resource: "Deployment", Namespace: "payments", Name: "checkout",
		Detail: strings.Repeat("x", detailBytes),
	}})
	require.NoError(t, err)
	loaded, err := manager.LoadChangeHistory(context.Background())
	require.NoError(t, err)
	require.Len(t, loaded, 1)
	require.LessOrEqual(t, len(loaded[0].Detail), maxChangeDetailBytes)
}
