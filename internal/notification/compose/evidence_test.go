package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

func TestWriteQuotesInvestigatedErrorBeforeLastOutput(t *testing.T) {
	d := incident.Decision{Action: incident.Announce,
		Incident: crashIncident(),
		Output:   []string{"panic: db down", "at main.go:12"},
		Evidence: []incident.Fact{
			{Kind: incident.FactError, Text: "panic: db down"},
		}}

	msg := Writer{}.Write(d, revisionNow)

	if !strings.Contains(msg.Note, `It fails with "panic: db down".`) {
		t.Fatalf("first error line should be quoted: %s", msg.Note)
	}
	if strings.Contains(msg.Note, "at main.go") {
		t.Fatalf("the last stack frame must not be quoted: %s", msg.Note)
	}
}

func TestWriteRendersEachInvestigatedFact(t *testing.T) {
	tests := []struct {
		name string
		fact incident.Fact
		want string
	}{
		{"termination", incident.Fact{Kind: incident.FactTermination,
			Text: "config missing"},
			`Its last run ended with "config missing".`},
		{"node", incident.Fact{Kind: incident.FactNode,
			Text: "MemoryPressure"}, "The node reports MemoryPressure."},
		{"top memory", incident.Fact{Kind: incident.FactTopMemory,
			Text: "etl-0 (3.1 GiB)"}, "using the most memory there are " +
			"etl-0 (3.1 GiB)."},
		{"scheduler", incident.Fact{Kind: incident.FactScheduler,
			Text: "Insufficient memory on 3 of 5 nodes"},
			"The scheduler rejects every node: Insufficient memory on " +
				"3 of 5 nodes."},
		{"keys", incident.Fact{Kind: incident.FactKeys,
			Subject: "configmap app", Text: "DB_HOST"},
			"The change touched keys DB_HOST of configmap app."},
		{"no endpoints", incident.Fact{Kind: incident.FactEndpoints,
			Subject: "hook", Text: "0"},
			"Service hook behind the webhook has no ready endpoints."},
		{"webhook", incident.Fact{Kind: incident.FactWebhook,
			Text: "failed calling webhook"},
			`The API server says "failed calling webhook".`},
		{"pull", incident.Fact{Kind: incident.FactPull, Text: "auth"},
			"The pull fails because the registry rejects the credentials."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := crashIncident()
			p.Root = inventory.CoreID("pod", "shop", "web-a")
			d := incident.Decision{Action: incident.Announce, Incident: p,
				Evidence: []incident.Fact{tt.fact}}

			msg := Writer{}.Write(d, revisionNow)

			if !strings.Contains(msg.Note, tt.want) {
				t.Fatalf("note %q does not contain %q", msg.Note, tt.want)
			}
		})
	}
}

func TestWriteUpdateQuotesOnlyFreshEvidence(t *testing.T) {
	d := incident.Decision{Action: incident.Update,
		Incident: crashIncident(), Reason: "material change"}

	without := Writer{}.Write(d, revisionNow)
	d.Evidence = []incident.Fact{
		{Kind: incident.FactError, Text: "panic: late"}}
	with := Writer{}.Write(d, revisionNow)

	if strings.Contains(without.Note, "panic: late") {
		t.Fatalf("no evidence must be quoted: %s", without.Note)
	}
	if !strings.Contains(with.Note, `"panic: late"`) {
		t.Fatalf("a new fact should join the update: %s", with.Note)
	}
}
