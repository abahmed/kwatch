package scenarios

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/pipeline/announce"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/rootcause"
)

var judgeStart = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

func entityID(kind, namespace, name string) inventory.EntityID {
	return inventory.EntityID{Kind: inventory.Kind(kind),
		Namespace: namespace, Name: name}
}

// caused is an incident rooted at root with a stated cause on root.
func causedIncident(
	incidentID string, root inventory.EntityID, score float64,
) incident.Incident {
	return incident.Incident{ID: incidentID, Root: root,
		Cause: &rootcause.CauseRecord{Root: root, Score: score}}
}

func announceDecision(p incident.Incident) incident.Decision {
	return incident.Decision{Action: incident.Announce, Incident: p}
}

func judgeAt(minutes int) time.Time {
	return judgeStart.Add(time.Duration(minutes) * time.Minute)
}

// Incidents listed in one roll-up were heard together: the order of their
// IDs, or a later message of one of them, must not make one of them first.
func TestReportedIncidentsDatesARollUpMemberByTheRollUp(t *testing.T) {
	t.Parallel()
	a := causedIncident("inc-1", entityID("service", "billing", "db-b"), 0.9)
	b := causedIncident("inc-2", entityID("deployment", "shop", "db-a"), 0.9)
	result := replay.Result{
		Decisions: []incident.Decision{{Action: incident.Update, Incident: b}},
		Times:     []time.Time{judgeAt(6)},
		Carried: []replay.CarriedDecision{
			{Decision: announceDecision(a), Carrier: "roll-up", At: judgeAt(1)},
			{Decision: announceDecision(b), Carrier: "roll-up", At: judgeAt(1)},
		},
		Incidents: []incident.Incident{a, b},
	}
	for _, p := range reportedIncidents(result) {
		if !p.First.Equal(judgeAt(1)) {
			t.Errorf("%s first heard %s, want the roll-up at %s",
				p.ID, p.First.Format("15:04"), judgeAt(1).Format("15:04"))
		}
	}
}

// Independent roots may be heard in any order, together or a minute
// apart: a label that lists them the other way round still matches both.
func TestRootCasesMatchEachRootInAnyOrder(t *testing.T) {
	t.Parallel()
	first := reported{ID: "inc-1", Root: "service/billing/db-b",
		Cause: &rootcause.CauseRecord{Score: 0.9}, First: judgeAt(1)}
	second := reported{ID: "inc-2", Root: "deployment/shop/db-a",
		Cause: &rootcause.CauseRecord{Score: 0.9}, First: judgeAt(2)}
	e := expectation{Name: "two", Root: "deployment/shop/db-a",
		OtherRoots: []string{"service/billing/db-b"}}
	for _, heard := range [][]reported{{first, second}, {second, first}} {
		for _, c := range rootCases(e, heard) {
			if !c.Correct {
				t.Errorf("%s: root %s not matched in order %s, %s",
					c.Name, c.Expected, heard[0].Root, heard[1].Root)
			}
		}
	}
}

// A single root is still judged against the first incident heard.
func TestRootCasesJudgeASingleRootAgainstTheFirstIncident(t *testing.T) {
	t.Parallel()
	wrong := reported{ID: "inc-1", Root: "node//n1", First: judgeAt(1),
		Cause: &rootcause.CauseRecord{Score: 0.8}}
	right := reported{ID: "inc-2", Root: "deployment/shop/api",
		First: judgeAt(2), Cause: &rootcause.CauseRecord{Score: 0.8}}
	e := expectation{Name: "one", Root: "deployment/shop/api"}
	cases := rootCases(e, []reported{wrong, right})
	if len(cases) != 1 || cases[0].Correct {
		t.Fatalf("cases = %+v, want one wrong case", cases)
	}
}

func TestRootCasesMarkHedgesAndFallbacks(t *testing.T) {
	t.Parallel()
	hedge := reported{ID: "inc-1", Root: "configmap/shop/flags", First: judgeAt(1),
		Cause: &rootcause.CauseRecord{Score: 0.69,
			Rival: &rootcause.CauseRecord{}}}
	e := expectation{Name: "two-causes", Root: rootUnknown, Ambiguous: true}
	c := rootCases(e, []reported{hedge})[0]
	if c.Correct || !c.Hedged || !c.Ambiguous || c.ExpectsRoot {
		t.Errorf("hedge case = %+v", c)
	}
	if c.WrongStated() {
		t.Error("a hedge on an ambiguous label is not a wrong stated root")
	}
	fallback := reported{ID: "inc-2", Root: "deployment/shop/api",
		First: judgeAt(1)}
	e = expectation{Name: "self", Root: "deployment/shop/api"}
	c = rootCases(e, []reported{fallback})[0]
	if !c.Correct || c.Stated() || !c.ExpectsRoot {
		t.Errorf("fallback case = %+v, want right, unstated, expecting a root",
			c)
	}
}

func TestWrongRootsAnnouncedJudgesEveryMessage(t *testing.T) {
	t.Parallel()
	node := entityID("node", "", "n1")
	api := entityID("deployment", "shop", "api")
	result := replay.Result{
		Decisions: []incident.Decision{
			announceDecision(causedIncident("inc-1", api, 0.8)),
			{Action: incident.Update, Incident: causedIncident("inc-1", node, 0.8)},
			{Action: incident.Resolve, Incident: causedIncident("inc-1", api, 0.8)},
		},
		Times: []time.Time{judgeAt(1), judgeAt(5), judgeAt(9)},
	}
	e := expectation{Name: "revised", Root: "node//n1"}
	first, overall := wrongRootsAnnounced(e, nil, result)
	if len(first) != 1 || first[0] != "deployment/shop/api" ||
		len(overall) != 1 {
		t.Fatalf("first = %v, overall = %v, want the first message wrong",
			first, overall)
	}
	v := verdict{expect: e, firstWrong: first, announcedWrong: overall}
	if !v.wrongFirstRoot() {
		t.Error("a wrong first root must count")
	}
	v.expect.RevisesRoot = true
	if v.wrongFirstRoot() {
		t.Error("a scenario labelled as a revision opts out")
	}
}

func TestWrongRootsAnnouncedAcceptsUnknownAndUnrelated(t *testing.T) {
	t.Parallel()
	api := entityID("deployment", "shop", "api")
	silent := incident.Incident{ID: "inc-1", Root: api}
	other := causedIncident("inc-2", entityID("node", "", "n2"), 0.9)
	result := replay.Result{
		Decisions: []incident.Decision{
			announceDecision(silent), announceDecision(other),
		},
		Times: []time.Time{judgeAt(1), judgeAt(2)},
	}
	e := expectation{Name: "calm", Root: "deployment/shop/api",
		Unrelated: []string{"node//n2"}}
	first, overall := wrongRootsAnnounced(e, nil, result)
	if len(first)+len(overall) != 0 {
		t.Errorf("first = %v, overall = %v: no cause says the cause is "+
			"unknown and an unrelated incident is not judged", first, overall)
	}
}

func TestWrongRootsAnnouncedNamesAHedgeRival(t *testing.T) {
	t.Parallel()
	rival := entityID("rolebinding", "shop", "web-config-reader")
	p := causedIncident("inc-1",
		entityID("rolebinding", "shop", "api-config-reader"), 0.69)
	p.Cause.Rival = &rootcause.CauseRecord{Root: rival, Score: 0.6}
	result := replay.Result{
		Decisions: []incident.Decision{announceDecision(p)},
		Times:     []time.Time{judgeAt(1)},
	}
	e := expectation{Name: "two-bindings",
		Root: "rolebinding/shop/api-config-reader"}
	_, overall := wrongRootsAnnounced(e, nil, result)
	if len(overall) != 1 || overall[0] != rival.String() {
		t.Errorf("overall = %v, want the rival %s named", overall, rival)
	}
	e.Ambiguous = true
	if _, overall = wrongRootsAnnounced(e, nil, result); len(overall) != 0 {
		t.Errorf("overall = %v: an ambiguous label tolerates the hedge",
			overall)
	}
}

func TestBlamedChecksTheRivalOfAHedge(t *testing.T) {
	t.Parallel()
	p := causedIncident("inc-1", entityID("node", "", "n1"), 0.49)
	p.Cause.Rival = &rootcause.CauseRecord{
		Root: entityID("deployment", "shop", "cart")}
	result := replay.Result{
		Decisions: []incident.Decision{announceDecision(p)},
		Times:     []time.Time{judgeAt(1)},
	}
	e := expectation{Root: "node//n1",
		MustNotBlame: []string{"deployment/shop/cart"}}
	if got := blamed(e, result); len(got) != 1 {
		t.Fatalf("blamed = %v, want the rival", got)
	}
}

// line builds one message line from spans written as "text", "*bold*" or
// "`code`".
func messageLine(spans ...notification.Span) notification.Block {
	return notification.Block{Kind: notification.Para, Spans: spans}
}

func plainSpan(text string) notification.Span {
	return notification.Span{Text: text}
}

func boldSpan(text string) notification.Span {
	return notification.Span{Text: text, Style: notification.Bold}
}

func codeSpan(text string) notification.Span {
	return notification.Span{Text: text, Style: notification.Code}
}

func TestBlamedReadsTheSentenceThatStatesTheCause(t *testing.T) {
	t.Parallel()
	subject := []notification.Span{boldSpan("api"), plainSpan(" in "),
		boldSpan("shop"), plainSpan(" ("), boldSpan(scenarioCluster),
		plainSpan(") ")}
	cases := map[string]struct {
		doc  notification.Block
		want int
	}{
		"names a cause after the tag": {messageLine(append(subject,
			plainSpan("is failing after the 10:01 deletion of role binding "),
			boldSpan("web-config-reader"), plainSpan("."))...), 1},
		"names a hedge": {messageLine(append(subject,
			plainSpan("keeps crashing; node "), boldSpan("web-config-reader"),
			plainSpan(" is what they share, which might be related."))...), 1},
		"is the subject only": {messageLine(append(subject,
			plainSpan("keeps crashing because its limit is too low."))...), 0},
		"context without a cause cue": {messageLine(plainSpan(
			"In the last 30 minutes in shop: role binding "),
			boldSpan("web-config-reader"),
			plainSpan(" was deleted.")), 0},
		"quoted output is code": {messageLine(append(subject,
			plainSpan("fails with "), codeSpan("because of web-config-reader"))...), 0},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			result := replay.Result{Messages: []notification.Message{
				{Doc: []notification.Block{c.doc}}}}
			e := expectation{Root: "deployment/shop/api",
				MustNotBlame: []string{"rolebinding/shop/web-config-reader"}}
			if got := blamed(e, result); len(got) != c.want {
				t.Errorf("blamed = %v, want %d", got, c.want)
			}
		})
	}
}

// A message sets a name without its kind: a Service named like the
// webhook it backs is not blamed when the webhook is the label's root.
func TestBlamedLeavesANameSharedWithAnAllowedRoot(t *testing.T) {
	t.Parallel()
	doc := messageLine(boldSpan("collector"), plainSpan(" in "),
		boldSpan("analytics"), plainSpan(" ("), boldSpan(scenarioCluster),
		plainSpan(") cannot create pods because validating webhook "),
		boldSpan("label-guard"), plainSpan(" times out."))
	result := replay.Result{Messages: []notification.Message{
		{Doc: []notification.Block{doc}}}}
	e := expectation{Root: "validatingwebhookconfiguration//label-guard",
		MustNotBlame: []string{"service/guard/label-guard"}}
	if got := blamed(e, result); len(got) != 0 {
		t.Errorf("blamed = %v, want none for a shared name", got)
	}
}

func TestBlamedIgnoresDroppedDecisions(t *testing.T) {
	t.Parallel()
	node := causedIncident("inc-1", entityID("node", "", "n1"), 0.9)
	result := replay.Result{Carried: []replay.CarriedDecision{{
		Decision: announceDecision(node), Carrier: announce.CarrierDropped,
		At: judgeAt(1)}}}
	e := expectation{Root: "deployment/shop/api",
		MustNotBlame: []string{"node//n1"}}
	if got := blamed(e, result); len(got) != 0 {
		t.Errorf("blamed = %v, a dropped decision was never heard", got)
	}
}

// A message about the pod of a labelled DaemonSet names the right thing at
// a finer grain: the log says the pod is owned by the DaemonSet.
func TestWrongRootsAnnouncedAllowsAnOwnedEntityOfTheLabelledRoot(t *testing.T) {
	t.Parallel()
	pod := entityID("pod", "kube-system", "kube-proxy-n3")
	set := entityID("daemonset", "kube-system", "kube-proxy")
	log := replay.Log{Start: judgeStart, Entries: []replay.Entry{{
		At: judgeStart,
		Observation: inventory.Observation{
			Kind: inventory.Related, Entity: pod,
			Relation: inventory.OwnedBy, Targets: []inventory.EntityID{set},
		},
	}}}
	result := replay.Result{
		Decisions: []incident.Decision{
			announceDecision(causedIncident("inc-1", pod, 0.7))},
		Times: []time.Time{judgeAt(1)},
	}
	e := expectation{Name: "agent", Root: set.String()}
	if first, _ := wrongRootsAnnounced(e, ownershipOf(log), result); len(
		first) != 0 {
		t.Errorf("first = %v: the pod belongs to the labelled DaemonSet",
			first)
	}
	if first, _ := wrongRootsAnnounced(e, nil, result); len(first) != 1 {
		t.Errorf("first = %v: without the log the pod is another entity",
			first)
	}
}
