package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func modelWith(
	t *testing.T, id inventory.EntityID, attrs map[string]inventory.Value,
) *inventory.Model {
	t.Helper()
	m := inventory.NewModel(inventory.Options{})
	_, err := m.Apply(inventory.Observation{Kind: inventory.Observed,
		Source: "test", At: time.Unix(0, 0), Entity: id, Attributes: attrs})
	require.NoError(t, err)
	return m
}

func TestCheckCauseReadsTheCausesStateNow(t *testing.T) {
	node := inventory.CoreID(kube.KindNode, "", "n3")
	db := inventory.CoreID(kube.KindExternalEndpoint, "", "db.example.com:5432")
	tt := []struct {
		name  string
		id    inventory.EntityID
		attrs map[string]inventory.Value
		want  CauseState
	}{
		{"node_ready", node, map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(true)}, CauseRecovered},
		{"node_not_ready", node, map[string]inventory.Value{
			kube.AttrReady: inventory.Bool(false)}, CauseStill},
		{"endpoint_ok", db, map[string]inventory.Value{
			kube.AttrHealthy: inventory.Bool(true)}, CauseRecovered},
		{"endpoint_refusing", db, map[string]inventory.Value{
			kube.AttrHealthy: inventory.Bool(false)}, CauseStill},
		{"endpoint_never_probed", db, nil, CauseUnchecked},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			m := modelWith(t, tc.id, tc.attrs)
			got := CheckCause(m, &rootcause.CauseRecord{Root: tc.id})
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestCheckCauseSaysNothingWhenItCannotLook(t *testing.T) {
	id := inventory.CoreID(kube.KindDeployment, "shop", "api")
	m := modelWith(t, id, nil)
	assert.Equal(t, CauseUnchecked, CheckCause(nil,
		&rootcause.CauseRecord{Root: id}))
	assert.Equal(t, CauseUnchecked, CheckCause(m, nil))
	assert.Equal(t, CauseUnchecked, CheckCause(m,
		&rootcause.CauseRecord{Root: id}), "a kind without a check")
	assert.Equal(t, CauseUnchecked, CheckCause(m, &rootcause.CauseRecord{
		Root: inventory.CoreID(kube.KindNode, "", "gone")}))
}
