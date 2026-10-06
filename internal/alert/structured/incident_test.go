package structured

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func decode(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	require.NoError(t, err)
	out := map[string]any{}
	require.NoError(t, json.Unmarshal(raw, &out))
	return out
}

func TestOrdinaryMessageHasNoFlagFields(t *testing.T) {
	got := decode(t, NewIncident("dev", providertest.Update()))
	for _, name := range []string{
		"pagingOnly", "skipPaging", "carrier", "reopenWithinSeconds",
	} {
		assert.NotContains(t, got, name)
	}
	assert.Equal(t, "p-42", got["key"])
}

func TestFlagsAreCarriedToTheReceiver(t *testing.T) {
	m := providertest.Resolve()
	m.Opens = true
	m.PagingOnly = true
	m.SkipPaging = true
	m.Carrier = "digest"
	m.ReopenWithin = 90 * time.Second

	got := decode(t, NewIncident("dev", m))
	assert.Equal(t, true, got["opens"])
	assert.Equal(t, true, got["pagingOnly"])
	assert.Equal(t, true, got["skipPaging"])
	assert.Equal(t, "digest", got["carrier"])
	assert.EqualValues(t, 90, got["reopenWithinSeconds"])
	assert.Equal(t, true, got["resolved"])
}
