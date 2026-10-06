package scenarios

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// recentChangeScenarios are failures nothing explains, where the
// message names what people changed next to them.
func recentChangeScenarios() []scenario {
	return []scenario{
		unknownCauseRecentChange(), unknownCauseOldChange(),
	}
}

// editedBy marks obj as last written by manager at the cluster's time.
func editedBy(c *cluster, obj metav1.Object, manager string) {
	at := metav1.NewTime(c.now)
	obj.SetManagedFields([]metav1.ManagedFieldsEntry{{
		Manager: manager, Time: &at,
		Operation: metav1.ManagedFieldsOperationUpdate,
	}})
}

// unknownCrash builds a crash loop of reports in shop that no object
// outside it explains, with an unrelated ConfigMap edited by bob
// editedAfter the start. The crash starts crashAfter the start.
func unknownCrash(c *cluster, editedAfter, crashAfter time.Duration) {
	flags := configMap(c, "shop", "feature-flags",
		map[string]string{"banner": "off"})
	w := c.deployment("shop", "reports",
		"registry.example.com/reports:3.4", 2)
	c.list(c.node("n1", "zone-a"), c.node("n2", "zone-a"), flags)
	c.list(w.objects())
	c.list(w.pod(0, "n1"), w.pod(1, "n2"))
	c.after(editedAfter)
	edited := last(c, flags)
	edited.Data["banner"] = "on"
	editedBy(c, edited, "bob")
	c.update(edited)
	c.after(crashAfter - editedAfter)
	for restarts := int32(2); restarts <= 6; restarts++ {
		c.update(w.pod(0, "n1", startedNow, crashLoop(2, "Error",
			"panic: nil pointer dereference in render()", restarts)))
		c.update(w.pod(1, "n2", startedNow, crashLoop(2, "Error",
			"panic: nil pointer dereference in render()", restarts)))
		c.after(time.Minute)
	}
}

// unknownCauseRecentChange: the app crash-loops with a panic of its own;
// nothing it uses changed. Nobody can be blamed, but the message lists
// the unrelated ConfigMap edit made minutes earlier as a fact.
func unknownCauseRecentChange() scenario {
	return scenario{
		expect: expectation{
			Name: "unknown-cause-recent-change",
			Description: "An app crash-loops with its own panic and no " +
				"cause is found; the message lists the recent change " +
				"in the namespace without blaming it.",
			Root: rootUnknown, Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"configmap/shop/feature-flags",
				"node//n1", "node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) { unknownCrash(c, time.Minute, 5*time.Minute) },
	}
}

// unknownCauseOldChange is the negative twin: the same crash, but the
// only edit is older than the 30-minute window, so the message names
// no change.
func unknownCauseOldChange() scenario {
	return scenario{
		expect: expectation{
			Name: "unknown-cause-old-change",
			Description: "The same crash loop when the namespace's " +
				"last edit is over 30 minutes old: no change is named.",
			Root: rootUnknown, Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"configmap/shop/feature-flags",
				"node//n1", "node//n2", "zone//zone-a"},
		},
		build: func(c *cluster) {
			unknownCrash(c, time.Minute, 45*time.Minute)
		},
	}
}
