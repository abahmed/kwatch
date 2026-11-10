package dashboard

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Each repeatable filter takes at most maxFilterValues values: more is
// a 400 that names the filter.
func TestListFilterValueCounts(t *testing.T) {
	s := newTestServer()
	for _, key := range []string{"namespace", "kind", "node", "mode"} {
		many := url.Values{}
		for i := 0; i < maxFilterValues+1; i++ {
			many.Add(key, "OOMKilled")
		}
		w := get(t, s, "/api/incidents?"+many.Encode())
		assert.Equal(t, http.StatusBadRequest, w.Code, key)
		assert.Contains(t, w.Body.String(), "Too many "+key, key)

		ok := url.Values{}
		for i := 0; i < maxFilterValues; i++ {
			ok.Add(key, "OOMKilled")
		}
		w = get(t, s, "/api/incidents?"+ok.Encode())
		assert.Equal(t, http.StatusOK, w.Code, key+" at the cap")
	}
}

func TestOldRouteKeepsTheReleaseFilter(t *testing.T) {
	u, _ := url.Parse("/?release=bad")
	assert.True(t, strings.HasPrefix(oldRoute(u), "/issues?release=bad"))
}
