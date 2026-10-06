package investigate

import (
	"context"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Investigator kinds.
const (
	kindAdmission  = "admission"
	kindRegistry   = "registry"
	kindConfig     = "config"
	kindNode       = "node"
	kindScheduling = "scheduling"
	kindCrash      = "crash"
)

// Investigation budgets. Reads from the model are fast; a read that
// calls the API gets more time. MaxBudget is the deadline of an
// investigation whose plan names no budget of its own, and the most any
// budget may take.
const (
	MaxBudget       = 5 * time.Second
	modelReadBudget = time.Second
	apiReadBudget   = 3 * time.Second
)

// investigator reads the facts that prove one kind of root cause.
type investigator struct {
	kind string
	// budget bounds the reads.
	budget  time.Duration
	matches func(incident.Incident) bool
	read    func(context.Context, Sources, incident.Incident) Result
}

// investigators are tried in order and the first match wins, so the
// specific kinds come before the broad crash kind: a node root with
// crashing pods is investigated as a node.
var investigators = []investigator{
	{kindAdmission, apiReadBudget, isAdmissionRoot, readAdmission},
	{kindRegistry, modelReadBudget, isRegistryRoot, readRegistry},
	{kindConfig, modelReadBudget, isConfigRoot, readConfig},
	{kindNode, modelReadBudget, isNodeRoot, readNode},
	{kindScheduling, modelReadBudget, isSchedulingRoot, readScheduling},
	{kindCrash, MaxBudget, isCrashRoot, readCrash},
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
		"Error", "CannotRun", "InitError", "Failed", "JobFailed",
		"Probe.Startup"}
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
