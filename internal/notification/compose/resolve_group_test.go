package compose

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// A group of workloads failing with one shared error recovers as that
// group: the resolve speaks of its workloads, never of an object a
// listing named before the group took its failures over, and never
// names the shared error by kwatch's own key.
func TestResolveOfASharedErrorGroupSpeaksOfItsWorkloads(t *testing.T) {
	signature := inventory.EntityID{Kind: explain.KindFailureSignature,
		Name: "unhandled exception redisconnectionexception"}
	var impact []inventory.EntityID
	for i := range 16 {
		impact = append(impact, inventory.CoreID(kube.KindDeployment,
			"apps", fmt.Sprintf("svc-%02d", i)))
	}
	p := incident.Incident{ID: "sig-1", Root: signature,
		Subject: inventory.CoreID(kube.KindService, "apps", "projects"),
		Impact:  impact, Tier: incident.Notify, State: incident.Resolved,
		Opened:   writerNow.Add(-20 * time.Minute),
		Resolved: writerNow}

	_, got := resolveNote(caseFacts{p: p, now: writerNow,
		asAnnounced: true})

	var text []string
	for _, s := range got {
		text = append(text, s.text)
	}
	all := strings.Join(text, " ")
	assert.True(t, strings.HasPrefix(all, "svc-00 in apps and 15 other "+
		"workloads are healthy again."), all)
	assert.NotContains(t, all, "failure-signature")
	assert.NotContains(t, all, "projects")
	assert.NotContains(t, all, "Last affected")
}
