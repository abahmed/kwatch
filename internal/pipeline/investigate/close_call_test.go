package investigate

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var (
	closeDB = inventory.CoreID(kube.KindExternalEndpoint, "",
		"db.example.com:5432")
	closeDNS = inventory.CoreID("cluster-dns", "", "cluster-dns")
)

func closeCallIncident() incident.Incident {
	return incident.Incident{Root: closeDB, Cause: &rootcause.CauseRecord{
		Root: closeDB, Score: 0.6,
		Rival: &rootcause.CauseRecord{Root: closeDNS, Score: 0.58}}}
}

func closeCallFacts(
	t *testing.T, result kube.CloseCallResult, p incident.Incident,
) Result {
	t.Helper()
	calls := 0
	sources := Sources{Model: testModel(t),
		CloseCall: func(context.Context, string) kube.CloseCallResult {
			calls++
			return result
		}}
	inv, ok := NewInvestigator(sources).Plan(p)
	require.True(t, ok)
	got := inv.Run(context.Background())
	if _, _, ok := closeCallEndpoint(p); ok {
		assert.Equal(t, 1, calls, "one check, no more")
	} else {
		assert.Zero(t, calls)
	}
	return got
}

func checkFact(r Result) (incident.Fact, bool) {
	for _, f := range r.Evidence {
		if f.Kind == incident.FactCheck {
			return f, true
		}
	}
	return incident.Fact{}, false
}

func TestCloseCallRulesOutDNSWhenTheNameResolves(t *testing.T) {
	got := closeCallFacts(t, kube.CloseCallResult{
		Endpoint: closeDB.Name, Host: "db.example.com", Resolved: true,
		Refused: true}, closeCallIncident())
	fact, ok := checkFact(got)
	require.True(t, ok)
	assert.Equal(t, "kwatch checked: DNS resolves db.example.com fine, "+
		"so DNS is not the cause.", fact.Text)
}

func TestCloseCallStatesOnlyTheFactOfAnInconclusiveCheck(t *testing.T) {
	got := closeCallFacts(t, kube.CloseCallResult{
		Endpoint: closeDB.Name, Host: "db.example.com", TimedOut: true},
		closeCallIncident())
	fact, _ := checkFact(got)
	assert.Equal(t, "kwatch checked db.example.com:5432: no answer in "+
		"5 seconds.", fact.Text)
	assert.NotContains(t, fact.Text, "so ")
}

func TestCloseCallNamesAHostThatDoesNotResolve(t *testing.T) {
	got := closeCallFacts(t, kube.CloseCallResult{
		Endpoint: closeDB.Name, Host: "db.example.com"},
		closeCallIncident())
	fact, _ := checkFact(got)
	assert.Equal(t, "kwatch checked: db.example.com does not resolve.",
		fact.Text)
}

func TestCloseCallSaysNothingForABlockedAddress(t *testing.T) {
	got := closeCallFacts(t, kube.CloseCallResult{
		Endpoint: closeDB.Name, Host: "db.example.com", Resolved: true,
		Blocked: true}, closeCallIncident())
	_, ok := checkFact(got)
	assert.False(t, ok, "kwatch did not dial it, so it found nothing")
}

func TestCloseCallDoesNotRunWithoutARivalOrAnEndpoint(t *testing.T) {
	alone := closeCallIncident()
	alone.Cause.Rival = nil
	assert.False(t, closeCallApplies(alone, nil))
	nodes := incident.Incident{Cause: &rootcause.CauseRecord{
		Root: inventory.CoreID(kube.KindNode, "", "n1"),
		Rival: &rootcause.CauseRecord{
			Root: inventory.CoreID(kube.KindNode, "", "n2")}}}
	assert.False(t, closeCallApplies(nodes, nil))
}
