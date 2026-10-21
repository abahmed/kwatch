package scenarios

import (
	"time"
)

// rcaPullScenarios are image pulls refused with an authentication error.
func rcaPullScenarios() []scenario {
	return []scenario{pullSecretRotatedHoursBefore(),
		pullWithoutSecretBesideLogins()}
}

// pullSecretRotatedHoursBefore: automation rotated a pull Secret to a
// token the registry rejects, three hours before anything noticed:
// running pods do not pull again. When a node joins and the ledger
// scales out, the new replica fails with 401. The rotation is the
// change that matters, long as it is before the failure.
func pullSecretRotatedHoursBefore() scenario {
	return scenario{
		expect: expectation{
			Name: "pull-secret-rotated-hours-before-scale-out",
			Description: "A pull Secret was rotated to a bad token " +
				"three hours before a scale-out finds out with 401.",
			Root: "secret/payments/pull-creds", Tier: "notify",
			MaxMessages: 2,
			MustNotBlame: []string{"deployment/payments/ledger",
				"node//p1", "node//p2", "node//p3"},
		},
		build: func(c *cluster) {
			c.list(c.node("p1", "zone-a"), c.node("p2", "zone-b"))
			ledger := heldoutPrivateApp(c, "payments", "ledger",
				"pull-creds")
			c.list(heldoutPullSecret(c, "payments", "pull-creds",
				"token-v7"))
			c.list(ledger.objects())
			c.list(ledger.pod(0, "p1"), ledger.pod(1, "p2"))
			c.after(time.Minute)
			c.update(heldoutPullSecret(c, "payments", "pull-creds",
				"token-v8"))
			c.after(3 * time.Hour)
			c.create(c.node("p3", "zone-a"))
			setReplicas(ledger, 3)
			ledger.setReady(2)
			c.update(ledger.objects())
			refused := "failed to authorize: failed to fetch anonymous " +
				"token: unexpected status from GET request to https://" +
				heldoutRegistry + "/v2/token: 401 Unauthorized"
			for attempt := range 6 {
				clusterPullFailure(c, ledger, 2, "p3", attempt, refused)
				c.after(40 * time.Second)
			}
		},
	}
}

// pullWithoutSecretBesideLogins: two new workloads of "labs" pull from
// the private registry and name no pull Secret, so the registry refuses
// them with 401, while the payments workloads pull from it fine with
// theirs. The registry accepts logins; the new workloads send none.
func pullWithoutSecretBesideLogins() scenario {
	return scenario{
		expect: expectation{
			Name: "pull-without-secret-beside-logins",
			Description: "Two workloads with no image pull Secret are " +
				"refused by a registry that other workloads pull " +
				"from with theirs.",
			Root:        "deployment/labs/newa",
			OtherRoots:  []string{"deployment/labs/newb"},
			Tier:        "notify",
			MaxMessages: 4,
			MustNotBlame: []string{"registry//" + heldoutRegistry,
				"node//p1", "node//p2"},
		},
		build: func(c *cluster) {
			c.list(c.node("p1", "zone-a"), c.node("p2", "zone-b"))
			c.list(heldoutPullSecret(c, "payments", "pull-creds", "tok"))
			for _, name := range []string{"ledger", "scorer"} {
				w := heldoutPrivateApp(c, "payments", name, "pull-creds")
				c.list(w.objects())
				c.list(w.pod(0, "p1"), w.pod(1, "p2"))
			}
			c.after(time.Minute)
			refused := "failed to authorize: failed to fetch anonymous " +
				"token: unexpected status from GET request to https://" +
				heldoutRegistry + "/v2/token: 401 Unauthorized"
			var fresh []*workload
			for _, name := range []string{"newa", "newb"} {
				w := c.deployment("labs", name,
					heldoutRegistry+"/labs/"+name+":1", 1)
				c.create(w.objects())
				fresh = append(fresh, w)
			}
			for attempt := range 6 {
				for _, w := range fresh {
					w.setReady(0)
					c.update(w.objects())
					clusterPullFailure(c, w, 0, "p1", attempt, refused)
				}
				c.after(40 * time.Second)
			}
		},
	}
}
