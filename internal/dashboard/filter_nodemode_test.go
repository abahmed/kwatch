package dashboard

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func pageIDs(t *testing.T, s *Server, path string) []string {
	t.Helper()
	w := get(t, s, path)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var got ListResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	out := []string{}
	for _, i := range got.Incidents {
		out = append(out, i.ID)
	}
	return out
}

func TestListFiltersByFailureFamily(t *testing.T) {
	pull := issueAt("pull", "a", incident.Notify, time.Hour)
	pull.Mode = "ImagePull.Registry"
	oom := issueAt("oom", "a", incident.Notify, time.Hour)
	oom.Mode = "OOMKilled"
	s := newTestServer(pull, oom)

	assert.Equal(t, []string{"pull"},
		pageIDs(t, s, "/api/incidents?mode=ImagePull"))
	assert.Equal(t, []string{"oom", "pull"}, pageIDs(t, s,
		"/api/incidents?mode=ImagePull&mode=OOMKilled&sort=title"))
	for _, bad := range []string{"Image.Pull", "a%20b", ""} {
		assert.Equal(t, http.StatusBadRequest,
			get(t, s, "/api/incidents?mode="+bad).Code, bad)
	}
}

func TestListFiltersByNode(t *testing.T) {
	tm := newTestModel(t)
	root, pod := tm.deployment()
	n1 := inventory.CoreID(kube.KindNode, "", "n1")
	tm.observe(n1, nil)
	tm.relate(pod, inventory.RunsOn, n1)
	on := issueAt("on", "shop", incident.Notify, time.Hour)
	on.Root = root
	off := issueAt("off", "web", incident.Notify, time.Hour)
	s := newTestServer(on, off)

	assert.Empty(t, pageIDs(t, s, "/api/incidents?node=n1"),
		"without the inventory nothing matches")
	s.SetModel(tm.m)
	assert.Equal(t, []string{"on"}, pageIDs(t, s, "/api/incidents?node=n1"))
	assert.Equal(t, http.StatusBadRequest,
		get(t, s, "/api/incidents?node=").Code)
}
