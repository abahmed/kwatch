package app

import (
	"context"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/status"
)

// upgradeTrialURL is where the edition notice sends people to try Pro.
const upgradeTrialURL = "https://kwatch.dev/signup?src=upgrade"

// stateEditionLast is the state key that records the edition the last
// start ran and whether the operator was told about a move to Community.
const stateEditionLast = "edition.last"

// detectorFamily is a group of Pro detectors shown under one name. The
// family runs when its first capability does.
type detectorFamily struct {
	name string
	ids  []feature.ID
}

// proDetectorFamilies lists, in the order they are shown, the detector
// capabilities the Community edition does not run. A test keeps it
// complete: every Pro capability of the core.* detectors is in one.
var proDetectorFamilies = []detectorFamily{
	{"workloads", []feature.ID{feature.WorkloadDetection,
		feature.WorkloadRollouts, feature.JobFailures,
		feature.DisruptionBudgets, feature.Autoscaling}},
	{"storage", []feature.ID{feature.StorageDetection, feature.VolumeUsage}},
	{"certificates", []feature.ID{feature.CertificateDetection}},
	{"network", []feature.ID{feature.NetworkDetection}},
	{"admission", []feature.ID{feature.AdmissionDetection}},
	{"quota", []feature.ID{feature.QuotaDetection}},
	{"custom resources", []feature.ID{feature.CustomResources}},
	{"control plane", []feature.ID{feature.ControlPlane}},
	{"node usage", []feature.ID{feature.NodeUsage}},
	{"pod readiness", []feature.ID{feature.PodReadiness}},
	{"configuration risks", []feature.ID{feature.ConfigurationRisks}},
	{"unusual events", []feature.ID{feature.UnusualEvents}},
}

// proDetectorsOff names the Pro detector families e does not run. Active
// probes are left out: they run only when configured, and
// configuredPro already reports that.
func proDetectorsOff(e feature.Entitlements) []string {
	e = feature.OrCommunity(e)
	var names []string
	for _, family := range proDetectorFamilies {
		if !e.Has(family.ids[0]) {
			names = append(names, family.name)
		}
	}
	return names
}

// editionNotice is what a Community start tells the operator about what
// it does not run: the Pro detector families and the provider cap.
type editionNotice struct {
	detectors []string
	sends     string
}

func newEditionNotice(
	e feature.Entitlements, cfg *config.Config,
) editionNotice {
	return editionNotice{
		detectors: proDetectorsOff(e), sends: communitySends(e, cfg),
	}
}

// lines are the notice as /status and lint word it.
func (n editionNotice) lines() []string {
	var out []string
	if len(n.detectors) > 0 {
		out = append(out, status.ProDetectorsText(n.detectors))
	}
	if n.sends != "" {
		out = append(out, status.CommunitySendsText(n.sends))
	}
	return out
}

// EditionNotices words what this installation's edition does not run,
// for `kwatch lint`: the Pro detector families and the provider kept.
func EditionNotices(cfg *config.Config) []string {
	return newEditionNotice(ResolveEntitlements(), cfg).lines()
}

// logEditionNotice logs, in one line, the Pro detector families a
// Community start does not run and the provider it keeps.
func logEditionNotice(e feature.Entitlements, n editionNotice) {
	if len(n.detectors) == 0 {
		return
	}
	args := []any{"edition", string(feature.OrCommunity(e).Edition()),
		"count", len(n.detectors), "detectors", n.detectors}
	if n.sends != "" {
		args = append(args, "communitySends", n.sends)
	}
	klog.InfoS("kwatch Pro detectors are not running in this edition",
		args...)
}

// editionRecord is the value under stateEditionLast.
type editionRecord struct {
	// Edition is the edition the last start ran.
	Edition string `json:"edition"`
	// Notified is true once the move to Community was announced.
	Notified bool `json:"notified,omitempty"`
}

// editionPlan is the decision of one start: the record to keep and
// whether the operator is told.
type editionPlan struct {
	record editionRecord
	notify bool
	known  bool
}

// planEdition decides from the persisted state whether this start runs
// Community after an earlier version or Pro. It reads the state before the
// startup bookkeeping marks the installation initialized, and writes
// nothing: commit does, once the notice can be sent.
func planEdition(
	ctx context.Context, disk diskState, e feature.Entitlements,
) (editionPlan, error) {
	edition := feature.OrCommunity(e).Edition()
	if edition == feature.ProEdition {
		return editionPlan{
			record: editionRecord{Edition: string(edition)}, known: true,
		}, nil
	}
	var last editionRecord
	found, err := disk.get(stateEditionLast, &last)
	if err != nil {
		return editionPlan{}, err
	}
	first, err := disk.IsFirstRun(ctx)
	if err != nil {
		return editionPlan{}, err
	}
	community := editionRecord{Edition: string(edition)}
	switch {
	case found && last.Edition == string(feature.ProEdition):
		community.Notified = true
		return editionPlan{record: community, notify: true, known: true}, nil
	case found:
		community.Notified = last.Notified
	case !first:
		// State from an earlier version, before editions existed.
		community.Notified = true
		return editionPlan{record: community, notify: true, known: true}, nil
	}
	return editionPlan{record: community, known: true}, nil
}

// commit records the plan, then sends the notice: the record comes first,
// so a failed write never repeats the message and a restart sends nothing.
func (p editionPlan) commit(
	disk diskState, send func(string), text string,
) {
	if !p.known {
		return
	}
	if err := disk.put(stateEditionLast, p.record); err != nil {
		klog.ErrorS(err, "record the edition", "component", "edition")
		return
	}
	if p.notify && send != nil {
		send(text)
	}
}

// upgradeNoticeText explains, once, what the Community edition keeps and
// what it does not run, and how to get it back.
func upgradeNoticeText(n editionNotice) string {
	text := notification.MarkerNotify + " kwatch is now running the " +
		"Community edition. It keeps the pod and node alerts with their " +
		"root cause; kwatch Pro restores the rest"
	if len(n.detectors) > 0 {
		text += ": " + status.ProDetectorsText(n.detectors)
	}
	text += ". Start a free Pro trial: " + upgradeTrialURL
	return text
}

// announceEdition sends the edition notice, once, after an earlier
// version or Pro. A fresh install hears nothing.
func announceEdition(deps *serverDeps, disk diskState, plan editionPlan) {
	var send func(string)
	if deps.deliveryManager != nil {
		send = deps.deliveryManager.Notify
	}
	plan.commit(disk, send, upgradeNoticeText(deps.notice))
}
