package compose

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// causeWording says in plain English what is wrong with a cause that
// has no finding of its own. Rule and mode identifiers never reach a
// reader; this table is where they become words.
type causeWording struct {
	// words follow the cause's name: "secret db-creds does not exist".
	words string
	// own marks a cause in the subject's own configuration: words are
	// then a whole clause about the subject ("its memory limit is too
	// low for normal use").
	own bool
}

// causeWords are keyed by the explain row that blamed the cause
// (rootcause.CauseRecord.Rule). A row missing here reads "is failing".
var causeWords = map[string]causeWording{
	"apiserver-unavailable":  {words: "is not answering"},
	"apiservice-unavailable": {words: "is unavailable"},
	"backends-failing":       {words: "has no healthy backends"},
	"autoscaling-limit": {
		words: "has reached its autoscaling limit"},
	"autoscaling-limit-unavailable": {
		words: "has reached its autoscaling limit"},
	"budget-blocks-drain":          {words: "allows no eviction"},
	"certificate-expired":          {words: "holds an expired certificate"},
	"claim-not-usable":             {words: "cannot be used"},
	"cluster-dns-failing":          {words: "is not resolving names"},
	"cluster-dns-servers":          {words: "is not resolving names"},
	"config-missing-or-changed":    {words: "does not exist"},
	"configmap-missing-or-changed": {words: "does not exist"},
	"controller-failing": {
		words: "is down, so nothing processes their changes"},
	"custom-resource-failing": {words: "is not ready"},
	"etcd-unavailable":        {words: "is unavailable"},
	// external-endpoint-failing is worded by endpointWords.
	"external-endpoint-failing": {words: "cannot be reached"},
	// helper-container-blocks-pod is worded by containerPhrase.
	"helper-container-blocks-pod": {words: "keeps failing and blocks its pod"},
	"memory-limit-too-low": {own: true,
		words: "its memory limit is too low for normal use"},
	"metrics-api-down":             {words: "is not serving metrics"},
	"namespace-terminating":        {words: "is being deleted"},
	"node-disk-pressure":           {words: "is low on disk"},
	"node-memory-pressure":         {words: "is low on memory"},
	"node-memory-pressure-unready": {words: "is low on memory"},
	"node-network":                 {words: "has lost its network"},
	"node-not-ready":               {words: "is not ready"},
	"node-pid-pressure":            {words: "is running out of process IDs"},
	"node-removed":                 {words: "was removed"},
	"node-removed-capacity": {
		words: "was removed and no other node has room"},
	"nodepool-failing": {words: "is failing as a whole"},
	"own-change":       {words: "changed shortly before"},
	"owner-failing":    {words: "is failing"},
	"policy-restricts": {words: "blocks its traffic"},
	"probe-port-mismatch": {own: true,
		words: "its probe checks a port the container does not listen on"},
	"quota-exhausted":  {words: "is used up"},
	"rbac-change":      {words: "no longer grants the access it needs"},
	"registry-refuses": {words: "refuses the image pulls"},
	"rollout":          {words: "rolled out a change shortly before"},
	"routed-missing":   {words: "does not exist"},
	// scheduler-capacity is worded by schedulingWords.
	"scheduler-capacity":    {words: "rejects every node"},
	"scheduler-unavailable": {words: "is down"},
	"self":                  {words: "is failing on its own"},
	"service-no-endpoints":  {words: "has no ready endpoints"},
	// shared-failure-signature is worded by signatureLead.
	"shared-failure-signature": {
		words: "is the error several workloads fail with"},
	"startup-budget-too-short": {own: true,
		words: "its containers get too little time to start"},
	"summary":                 {words: "is failing"},
	"used-missing":            {words: "does not exist"},
	"webhook-backend-missing": {words: "does not exist"},
	"webhook-rejects":         {words: "is rejecting requests"},
	"zone-failing":            {words: "is failing as a whole"},
}

// ownCause reports a cause worded as a clause about the subject.
func ownCause(cause *rootcause.CauseRecord) bool {
	return cause != nil && causeWords[cause.Rule].own
}

// causeWordsFor is what follows the cause's name when it has no
// finding of its own.
func causeWordsFor(cause *rootcause.CauseRecord) string {
	if cause.Root.Kind == kube.KindRegistry {
		if words, ok := pullWords[strings.ToLower(string(cause.Mode))]; ok {
			return strings.TrimPrefix(words, "the registry ")
		}
	}
	if cause.Root.Kind == explain.KindExternalEndpoint {
		return endpointWords(cause.Mode)
	}
	if claimFull(cause) {
		return "is out of space"
	}
	if words, ok := webhookCallWords[cause.Mode]; ok {
		return words
	}
	if wording, ok := causeWords[cause.Rule]; ok {
		return wording.words
	}
	return "is failing"
}

// webhookCallWords say how a webhook the API server could not call
// fails, by explain's pseudo mode.
var webhookCallWords = map[detection.Mode]string{
	explain.ModeWebhookTimeout:    "times out on every call",
	explain.ModeWebhookCallFailed: "cannot be called",
}

// claimFull reports a volume claim blamed because it has no space left.
func claimFull(cause *rootcause.CauseRecord) bool {
	return cause != nil && cause.Root.Kind == kube.KindPVC &&
		cause.Mode == detection.ModeVolumeFull
}

// knownRule reports a cause whose rule has words in causeWords.
func knownRule(cause *rootcause.CauseRecord) bool {
	_, ok := causeWords[cause.Rule]
	return ok
}

// readableSummary reports a cause summary written for people. Explain
// always names its row, and its summary is for traces; a cause without
// a row was written by hand, and its summary is what to say.
func readableSummary(cause *rootcause.CauseRecord) bool {
	return cause.Rule == "" && cause.Summary != ""
}

// schedulingWords name a cluster-wide scheduling blocker, whose entity
// name is the scheduler's reason: "Insufficient cpu" becomes "no node
// has enough cpu".
func schedulingWords(id inventory.EntityID) string {
	if resource, ok := strings.CutPrefix(id.Name, "Insufficient "); ok {
		return "no node has enough " + resource
	}
	return "the scheduler rejects every node (" + id.Name + ")"
}

// proofWords word one piece of evidence from its code and numbers. A
// proof without a code is written as its text; an empty result drops
// it.
func proofWords(p rootcause.Proof) string {
	if p.Code == "" {
		return p.Text
	}
	if write, ok := proofWriters[p.Code]; ok {
		return write(p)
	}
	return proofPhrases[p.Code]
}

// proofWriters word the proofs whose numbers or fields shape the
// sentence.
var proofWriters = map[rootcause.ProofCode]func(rootcause.Proof) string{
	rootcause.ProofDependentsFail: func(p rootcause.Proof) string {
		return dependentWords(p.Count, p.Total)
	},
	rootcause.ProofErrorsName: func(p rootcause.Proof) string {
		return errorWords(p.Count, p.Total)
	},
	rootcause.ProofWorkloadsMeet: func(p rootcause.Proof) string {
		return numberWord(p.Count) + " failing workloads all depend on it"
	},
	rootcause.ProofChanged: func(p rootcause.Proof) string {
		return changedFieldsWords(p.Fields)
	},
	rootcause.ProofBaseline: func(p rootcause.Proof) string {
		return "it deviates " + strconv.Itoa(p.Count) +
			"% from its baseline"
	},
}

// proofPhrases word the proofs that need no numbers. A code missing
// here and in proofWriters is dropped: it only ever speaks against a
// cause, and writers show supporting proof only.
var proofPhrases = map[rootcause.ProofCode]string{
	// Timing alone convinces nobody reading the note.
	rootcause.ProofBeganBefore:     "",
	rootcause.ProofNothingUpstream: "nothing it depends on is failing",
	rootcause.ProofCreated:         "it was created shortly before",
	rootcause.ProofSiblingsHealthy: "replicas that do not depend on it " +
		"are healthy",
	rootcause.ProofRevertRecovered: "reverting its change brought its " +
		"dependents back",
	rootcause.ProofFailedAfterChange: "its dependents failed soon after " +
		"it changed",
	rootcause.ProofNewRevisionFails: "only the new revision fails; the " +
		"previous one stays healthy",
}

// dependentWords writes "its only dependent is failing", "all of its
// dependents are failing" or "two of its three dependents are failing".
func dependentWords(n, of int) string {
	switch {
	case of == 1 && n == 1:
		return "its only dependent is failing"
	case n == of:
		return "all of its dependents are failing"
	}
	return numberWord(n) + " of its " + numberWord(of) + " dependents " +
		verb(n, "is", "are") + " failing"
}

// errorWords writes "the error mentions it", "every error mentions it"
// or "two of the four errors mention it".
func errorWords(n, of int) string {
	switch {
	case of == 1 && n == 1:
		return "the error mentions it"
	case n == of:
		return "every error mentions it"
	}
	return numberWord(n) + " of the " + numberWord(of) + " errors " +
		verb(n, "mentions", "mention") + " it"
}

// changedFieldsWords words a change by the fields it touched: "its
// selector changed shortly before".
func changedFieldsWords(paths []string) string {
	if len(paths) == 0 {
		return "it changed shortly before"
	}
	var fields []string
	for _, path := range paths {
		fields = append(fields, lastSegment(path))
	}
	return "its " + joinAnd(fields, maxNamed) + " changed shortly before"
}
