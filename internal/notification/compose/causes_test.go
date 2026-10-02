package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// TestCauseWordsCoverEveryRow keeps the wording table complete: a new
// explain row must say in plain English what it blames.
func TestCauseWordsCoverEveryRow(t *testing.T) {
	for _, row := range explain.Table() {
		if _, ok := causeWords[row.Name]; !ok {
			t.Errorf("row %q has no entry in causeWords", row.Name)
		}
	}
}

// TestCausePhraseNeverShowsIdentifiers checks a cause without findings
// is worded from the table, never from explain's summary.
func TestCausePhraseNeverShowsIdentifiers(t *testing.T) {
	cart := inventory.CoreID(kube.KindDeployment, "shop", "cart")
	secret := inventory.CoreID(kube.KindSecret, "shop", "db")
	cases := map[string]struct {
		cause   rootcause.CauseRecord
		subject inventory.EntityID
		want    string
	}{
		"missing secret": {rootcause.CauseRecord{
			Rule: "config-missing-or-changed", Mode: explain.ModeMissing,
			Root: secret, Summary: "secret db (Missing) explains 2 failures"},
			cart, "secret db does not exist"},
		"own memory limit": {rootcause.CauseRecord{
			Rule: "memory-limit-too-low", Mode: explain.ModeMemoryTooLow,
			Root: cart, Summary: "deployment cart " +
				"(Config.MemoryLimitTooLow) explains 2 failures"},
			cart, "its memory limit is too low for normal use"},
		"registry credentials": {rootcause.CauseRecord{
			Rule: "registry-refuses", Mode: "Auth",
			Root:    inventory.CoreID(kube.KindRegistry, "", "r.example"),
			Summary: "registry r.example (Auth) explains 3 failures"},
			cart, "registry r.example rejects the credentials"},
		"scheduling": {rootcause.CauseRecord{Rule: "scheduler-capacity",
			Root: inventory.CoreID(rootcause.KindScheduling, "",
				"Insufficient cpu")}, cart, "no node has enough cpu"},
		"full claim": {rootcause.CauseRecord{Rule: "claim-not-usable",
			Mode: detection.ModeVolumeFull,
			Root: inventory.CoreID(kube.KindPVC, "shop", "data"),
			Summary: "persistentvolumeclaim data (VolumeFull) explains " +
				"2 failures"}, cart, "volume claim data is out of space"},
		"webhook timing out": {rootcause.CauseRecord{Rule: "webhook-rejects",
			Mode: explain.ModeWebhookTimeout,
			Root: inventory.CoreID(kube.KindValidatingHook, "", "p"),
			Summary: "validatingwebhookconfiguration p (Webhook.Timeout) " +
				"explains 2 failures"},
			cart, "validating webhook p times out on every call"},
		"autoscaling ceiling": {rootcause.CauseRecord{
			Rule: "autoscaling-limit",
			Root: inventory.CoreID(kube.KindHPA, "shop", "cart")}, cart,
			"autoscaler cart has reached its autoscaling limit"},
		"unknown row": {rootcause.CauseRecord{Rule: "new-row", Mode: "Odd",
			Root: secret, Summary: "secret db (Odd) explains 1 failure"},
			cart, "secret db is failing"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got := causePhrase(&c.cause, c.subject)
			if got != c.want {
				t.Errorf("causePhrase = %q, want %q", got, c.want)
			}
			if strings.Contains(got, "explains") ||
				strings.Contains(got, "(") {
				t.Errorf("identifier leaked: %q", got)
			}
		})
	}
}

// TestProofWordsReadLikePeople words each kind of evidence from its
// code and numbers.
func TestProofWordsReadLikePeople(t *testing.T) {
	cases := map[string]struct {
		proof rootcause.Proof
		want  string
	}{
		"every dependent": {rootcause.Proof{
			Code: rootcause.ProofDependentsFail, Count: 2, Total: 2},
			"all of its dependents are failing"},
		"only dependent": {rootcause.Proof{
			Code: rootcause.ProofDependentsFail, Count: 1, Total: 1},
			"its only dependent is failing"},
		"some dependents": {rootcause.Proof{
			Code: rootcause.ProofDependentsFail, Count: 1, Total: 3},
			"one of its three dependents is failing"},
		"every error": {rootcause.Proof{
			Code: rootcause.ProofErrorsName, Count: 3, Total: 3},
			"every error mentions it"},
		"some errors": {rootcause.Proof{
			Code: rootcause.ProofErrorsName, Count: 1, Total: 2},
			"one of the two errors mentions it"},
		"workloads meet": {rootcause.Proof{
			Code: rootcause.ProofWorkloadsMeet, Count: 3},
			"three failing workloads all depend on it"},
		"changed fields": {rootcause.Proof{Code: rootcause.ProofChanged,
			Fields: []string{"spec.selector"}},
			"its selector changed shortly before"},
		"changed": {rootcause.Proof{Code: rootcause.ProofChanged},
			"it changed shortly before"},
		"created": {rootcause.Proof{Code: rootcause.ProofCreated},
			"it was created shortly before"},
		"timing": {rootcause.Proof{Code: rootcause.ProofBeganBefore}, ""},
		"baseline": {rootcause.Proof{Code: rootcause.ProofBaseline,
			Count: 80}, "it deviates 80% from its baseline"},
		"written by hand": {rootcause.Proof{
			Text: "only the new revision fails"},
			"only the new revision fails"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := proofWords(c.proof); got != c.want {
				t.Errorf("proofWords = %q, want %q", got, c.want)
			}
		})
	}
}
