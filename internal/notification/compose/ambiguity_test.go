package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Names from several namespaces always say their namespace, the lead's
// own included, and groups are told apart by semicolons: "app1 in
// billing, app0 and app1 in media" reads as three names in media.
func TestNameListSaysEveryNamespaceWhenThereAreSeveral(t *testing.T) {
	home := inventory.CoreID(kube.KindDeployment, "shop", "api")
	dep := func(ns, name string) inventory.EntityID {
		return inventory.CoreID(kube.KindDeployment, ns, name)
	}

	got := nameList(home, []inventory.EntityID{dep("billing", "app1"),
		dep("media", "app0"), dep("media", "app1")})
	assert.Equal(t, "app1 in billing; app0 and app1 in media", got)

	got = nameList(home, []inventory.EntityID{dep("shop", "cart"),
		dep("media", "app0")})
	assert.Equal(t, "cart in shop; app0 in media", got)

	got = nameList(home, []inventory.EntityID{dep("shop", "a"),
		dep("media", "b"), dep("ops", "c"), dep("ops", "d")})
	assert.Equal(t, "a in shop; b in media; c in ops; and one other", got)
}

func TestNameListKeepsTheLeadsNamespaceImplicitWhenAlone(t *testing.T) {
	home := inventory.CoreID(kube.KindDeployment, "shop", "api")
	dep := func(name string) inventory.EntityID {
		return inventory.CoreID(kube.KindDeployment, "shop", name)
	}

	assert.Equal(t, "cart and web",
		nameList(home, []inventory.EntityID{dep("cart"), dep("web")}))
}

// A policy that blocks a call leads with the caller, which is a
// workload: its name keeps its case, it is not listed as affected, and
// the one error it wrote is quoted once.
func TestPolicyBlockLeadReadsAsTheCallerNotAsTheSubjectAffected(t *testing.T) {
	policy := inventory.CoreID(kube.KindNetworkPolicy, "shop", "deny-all")
	orders := inventory.CoreID(kube.KindDeployment, "shop", "orders")
	crash := inventory.CoreID(kube.KindContainer, "shop",
		"orders-f2dc-a/app")
	line := "FATAL: could not connect: dial tcp 10.96.14.3:5432: i/o timeout"
	p := incident.Incident{ID: "inc-1", Root: policy, Tier: incident.Notify,
		State: incident.Open, Opened: at(0, 0), Revision: 1,
		Impact: []inventory.EntityID{orders},
		Cause: &rootcause.CauseRecord{Rule: "policy-blocks-call",
			Root: policy, Mode: "BlocksCall.postgres:5432"},
		Members: members(detection.Finding{Entity: crash,
			Reason: reasons.CrashLoopBackOff, Severity: detection.Critical,
			Since: at(0, 0), Summary: "Container is crash looping",
			Evidence: []detection.Evidence{
				{Label: "error", Value: line}}})}

	got := Writer{}.Write(announce(p), at(6, 0)).Note

	assert.Contains(t, got, "orders in shop can't reach postgres:5432")
	assert.NotContains(t, got, "Orders in shop")
	assert.NotContains(t, got, "orders is affected")
	assert.Equal(t, 1, strings.Count(got, line), got)
}

// Two members that report the same baseline figure give one phrase,
// and each figure keeps the verdict of the finding it came from.
func TestBaselineKeepsEachPhrasesOwnVerdictOnce(t *testing.T) {
	a := inventory.CoreID(kube.KindContainer, "shop", "api-1/app")
	b := inventory.CoreID(kube.KindContainer, "shop", "api-2/app")
	finding := func(id inventory.EntityID, n detection.Normality,
		phrase string,
	) detection.Finding {
		return detection.Finding{Entity: id, Reason: reasons.HighRestartCount,
			Severity: detection.Warning, Since: at(0, 0), Normal: n,
			Summary: "Container keeps restarting",
			Evidence: []detection.Evidence{{
				Label: detection.EvidenceBaseline, Value: phrase}}}
	}
	f := caseFacts{ok: true, members: []detection.Finding{
		finding(a, detection.NormalUnusual, "restarts 9×/h vs a usual 0.1×/h"),
		finding(b, detection.NormalUnusual, "restarts 9×/h vs a usual 0.1×/h"),
		finding(a, detection.NormalUsual, "restarts 2×/h vs a usual 2×/h"),
	}}

	got := baselineSentences(f)

	var texts []string
	for _, s := range got {
		texts = append(texts, s.text)
	}
	assert.Equal(t, []string{
		"That is unusual for this workload: restarts 9×/h vs a usual " +
			"0.1×/h.",
		"That is within what this workload normally does: restarts " +
			"2×/h vs a usual 2×/h."}, texts)
}

// A container and its pod recovering together is one recovery, told
// once, with what is still open.
func TestUpdateTellsAContainerAndItsPodRecoveringOnce(t *testing.T) {
	p := badRollout()
	p.Revision = 2
	pod := inventory.CoreID(kube.KindPod, "shop", "payments-7d9f")
	p.Timeline = []incident.Event{
		{At: at(2, 20), Text: "Container is crash looping"},
		{At: at(9, 0), Text: incident.RecoveredPrefix + "Container ok",
			Entity: aboutEntity(paymentsCt)},
		{At: at(9, 0), Text: incident.RecoveredPrefix + "Pod ready",
			Entity: aboutEntity(pod)},
	}
	p.Reported, p.ReportedKnown = 1, true
	d := incident.Decision{Action: incident.Update, Incident: p,
		Reason: "material change"}

	note := Writer{}.Write(d, at(9, 0)).Note

	assert.Equal(t, 1, strings.Count(note, "has recovered"), note)
	assert.Equal(t, 1, strings.Count(note, "still has"), note)
	assert.Contains(t, note, "Pod payments-7d9f has recovered")
}
