package investigate

import (
	"context"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Investigation budgets. Reads from the model are fast; a read that
// calls the API gets more time. MaxBudget is the deadline of a whole
// plan, and the most any budget may take.
const (
	MaxBudget       = 10 * time.Second
	modelReadBudget = time.Second
	apiReadBudget   = 3 * time.Second
	crashReadBudget = 5 * time.Second
)

// maxChecks bounds the checks one plan runs; the registry order decides
// which run first.
const maxChecks = 10

// check reads one kind of evidence. applies decides whether it runs for
// an incident; leads, when set, whether it may give the channel summary,
// and summary reads that summary. A check with a summary reports it as
// its section too, so nothing is read twice.
type check struct {
	name  string
	title string
	// budget bounds the reads.
	budget  time.Duration
	applies func(incident.Incident, inventory.Reader) bool
	read    func(context.Context, Sources, incident.Incident) []incident.Section
	leads   func(incident.Incident) bool
	summary func(context.Context, Sources, incident.Incident) Result
}

// checks are in priority order. The root-kind investigators come first
// and the first that leads gives the summary, so the specific kinds come
// before the broad crash kind: a node root with crashing pods is
// summarized as a node. The report checks follow.
var checks = append(leadChecks(), reportChecks()...)

// leadChecks are the root-kind investigators. Each runs when it leads.
func leadChecks() []check {
	return []check{
		leader(incident.CheckAdmission, "Webhook", apiReadBudget,
			isAdmissionRoot, readAdmission),
		leader(incident.CheckRegistry, "Image pull", modelReadBudget,
			isRegistryRoot, readRegistry),
		withRead(leader(incident.CheckConfig, "Secrets and ConfigMaps",
			modelReadBudget, isConfigRoot, readConfig), readConfigKeys),
		nodeCheck(),
		leader(incident.CheckScheduling, "Scheduling", modelReadBudget,
			isSchedulingRoot, readScheduling),
		leader(incident.CheckCrash, "Crash output", crashReadBudget,
			isCrashRoot, readCrash),
		leader(incident.CheckInitWait, "Init container output",
			apiReadBudget, isInitWaitRoot, readInitWait),
	}
}

// leader is a check that runs exactly when it may lead.
func leader(
	name, title string, budget time.Duration,
	leads func(incident.Incident) bool,
	summary func(context.Context, Sources, incident.Incident) Result,
) check {
	return check{name: name, title: title, budget: budget,
		applies: func(p incident.Incident, _ inventory.Reader) bool {
			return leads(p)
		}, leads: leads, summary: summary}
}

// withRead adds a report read to a leading check; its facts join the
// check's own section.
func withRead(
	c check,
	read func(context.Context, Sources, incident.Incident) []incident.Section,
) check {
	c.read = read
	return c
}

// applicable returns the checks that apply to p, at most maxChecks, in
// priority order.
func applicable(p incident.Incident, model inventory.Reader) []check {
	var out []check
	for _, c := range checks {
		if len(out) < maxChecks && c.applies(p, model) {
			out = append(out, c)
		}
	}
	return out
}

// leadOf is the index of the first check that leads p, or -1.
func leadOf(p incident.Incident, planned []check) int {
	for i, c := range planned {
		if c.leads != nil && c.leads(p) {
			return i
		}
	}
	return -1
}

// Failure modes by investigator kind. A mode matches its own name and
// every finer mode: "CrashLoop" matches "CrashLoop.HighFrequency".
var (
	admissionModes = []string{"Webhook", "InvalidPolicy"}
	registryModes  = []string{"ImagePull"}
	configModes    = []string{"CreateError.Config", "Missing.Secret",
		"Missing.ConfigMap"}
	schedulingModes = []string{"Unschedulable", "Pending", "SchedulingGated"}
	crashModes      = []string{"CrashLoop", "OOMKilled", "Restarting",
		"Error", "InitError", "Failed", "JobFailed", "Probe.Startup"}
)

func isAdmissionRoot(p incident.Incident) bool {
	return p.Root.Kind == kube.KindMutatingWebhook ||
		p.Root.Kind == kube.KindValidatingHook ||
		hasMode(p, admissionModes)
}

func isRegistryRoot(p incident.Incident) bool {
	return p.Root.Kind == kube.KindRegistry || hasMode(p, registryModes)
}

func isConfigRoot(p incident.Incident) bool {
	return isConfigKind(p.Root.Kind) || hasMode(p, configModes)
}

func isNodeRoot(p incident.Incident) bool {
	return p.Root.Kind == kube.KindNode
}

func isSchedulingRoot(p incident.Incident) bool {
	return hasMode(p, schedulingModes)
}

func isCrashRoot(p incident.Incident) bool {
	return hasMode(p, crashModes)
}

func isConfigKind(kind inventory.Kind) bool {
	return kind == kube.KindConfigMap || kind == kube.KindSecret
}

// hasMode reports whether the incident or one of its members fails in
// one of modes.
func hasMode(p incident.Incident, modes []string) bool {
	if modeIn(string(p.Mode), modes) {
		return true
	}
	for _, m := range p.Members {
		if modeIn(string(m.Mode), modes) {
			return true
		}
	}
	return false
}

func modeIn(mode string, modes []string) bool {
	for _, m := range modes {
		if mode == m || strings.HasPrefix(mode, m+".") {
			return true
		}
	}
	return false
}
