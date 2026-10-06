package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// Text a pod wrote is quoted as written: the kind spelling, tense and
// plural rewrites only touch the sentences kwatch built.
func TestRewritesLeaveQuotedPodTextAlone(t *testing.T) {
	names := map[inventory.Kind]string{"certificate": "Certificate"}
	in := []sentence{{part: partProof, text: `certificate web fails with ` +
		`"x509: certificate signed by unknown authority"`}}

	got := respell(in, names)[0].text

	want := `Certificate web fails with ` +
		`"x509: certificate signed by unknown authority"`
	if got != want {
		t.Errorf("respell = %q, want %q", got, want)
	}
	if got := pastTense(`n1 is low and says "it is bad"`); got !=
		`n1 was low and says "it is bad"` {
		t.Errorf("pastTense = %q", got)
	}
	if got := pastTense(`says "it is bad" and is low`); got !=
		`says "it is bad" and was low` {
		t.Errorf("pastTense (late) = %q", got)
	}
	if got := pluralPredicate(`is stuck on "its disk"`); got !=
		`are stuck on "its disk"` {
		t.Errorf("pluralPredicate = %q", got)
	}
	if got := pluralPredicate(`has not got its disk`); got !=
		`have not got their disk` {
		t.Errorf("pluralPredicate plain = %q", got)
	}
}

func TestPredicateAndContainerPhraseNeverEndEmpty(t *testing.T) {
	id := inventory.CoreID("container", "shop", "web-1/migrate")
	if got := predicate(id, ""); got != "is failing" {
		t.Errorf("predicate of nothing = %q", got)
	}
	got := containerPhrase(id, "Has not started")
	if got != strings.TrimSpace(got) || !strings.Contains(got, "has not") {
		t.Errorf("containerPhrase = %q", got)
	}
}

func TestOutageLeadCountsOneAndNone(t *testing.T) {
	o := NamespaceOutage{Namespace: "shop"}
	one := []incident.Decision{{Incident: incident.Incident{
		Opened: writerNow}}}
	if got := outageLead(o, "", one); got !=
		"shop: 1 workload failing since 10:00" {
		t.Errorf("one = %q", got)
	}
	got := outageLead(o, "", nil)
	if strings.Contains(got, "since") || strings.Contains(got, "00:00") {
		t.Errorf("none = %q", got)
	}
}

func TestMinutesWordsUnderAMinute(t *testing.T) {
	cases := map[time.Duration]string{
		10 * time.Second: "under a minute",
		90 * time.Second: "2 minutes",
		time.Minute:      "1 minute",
		5 * time.Minute:  "5 minutes",
	}
	for d, want := range cases {
		if got := minutesWords(d); got != want {
			t.Errorf("minutesWords(%s) = %q, want %q", d, got, want)
		}
	}
}

// Every quote loses its credentials. Private addresses stay: they are
// no secret and help whoever debugs the failure.
func TestQuotesHideSecretsButKeepPrivateAddresses(t *testing.T) {
	got := quoted("dial tcp 10.1.2.3:5432: refused, password=hunter2")
	if strings.Contains(got, "hunter2") {
		t.Errorf("quote %q leaks the password", got)
	}
	if !strings.Contains(got, "10.1.2.3:5432") {
		t.Errorf("quote %q should keep the private address", got)
	}
}

// An incident ended by the still-broken cap does not claim health.
func TestResolveAtStillBrokenCapDoesNotClaimHealth(t *testing.T) {
	p := podCrash(incident.Notify)
	p.State, p.Resolved = incident.Resolved, writerNow
	d := incident.Decision{Action: incident.Resolve, Incident: p,
		Reason: incident.Reason(incident.StoppedTrackingPrefix +
			"2h; coverage check continues")}

	msg := Writer{}.Write(d, writerNow)

	if strings.Contains(msg.Note, "healthy") ||
		!strings.Contains(msg.Note, "stopped tracking") ||
		!strings.Contains(msg.Note, "still") {
		t.Errorf("note = %q", msg.Note)
	}
}

// A summary carries the routes of the problems it names, so a provider
// routed by reason or namespace still gets the summary that holds one.
func TestSummariesCarryTheRoutesOfTheirMembers(t *testing.T) {
	d := announce(podCrash(incident.Notify))
	other := announce(podCrash(incident.Page))
	other.Incident.Root = inventory.CoreID("pod", "billing", "b-1")
	for key, f := range other.Incident.Members {
		f.Entity = other.Incident.Root
		f.Reason = "OOMKilled"
		delete(other.Incident.Members, key)
		other.Incident.Members[f.Key()] = f
	}
	decisions := []incident.Decision{d, other}

	routes := map[string][]notification.Route{
		"startup": {Writer{}.StartupSummary(decisions, writerNow).Route},
		"rollup":  {Writer{}.Rollup(decisions, writerNow).Route},
		"outage": {Writer{}.NamespaceOutage(NamespaceOutage{Namespace: "x"},
			decisions, writerNow).Route},
		"restored": {Writer{}.RestoredSummary(decisions, writerNow).Route},
		"digest":   {Writer{}.Digest(decisions, nil, nil, writerNow).Route},
	}
	for name, got := range routes {
		r := got[0]
		if len(r.AnyOf) != 2 || r.Severity != "critical" {
			t.Fatalf("%s route = %+v", name, r)
		}
		found := false
		for _, alt := range r.AnyOf {
			found = found || (alt.Reasons[0] == "OOMKilled" &&
				alt.Namespaces[0] == "billing")
		}
		if !found {
			t.Errorf("%s alternatives = %+v", name, r.AnyOf)
		}
	}
	if r := (Writer{}).StartupResolved("startup/1", 2).Route; r.AnyOf != nil {
		t.Errorf("a closing message names no problems: %+v", r)
	}
}
