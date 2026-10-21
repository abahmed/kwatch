package app

import (
	"context"
	"strings"

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
	// inactive names the Pro features the configuration sets that the
	// edition does not run (see inactivePro).
	inactive []string
	// droppedPagers are the paging services the provider cap leaves
	// inactive: nobody is paged through them.
	droppedPagers []string
}

func newEditionNotice(
	e feature.Entitlements, cfg *config.Config,
) editionNotice {
	_, dropped := capProviders(e, cfg)
	return editionNotice{
		detectors: proDetectorsOff(e), sends: communitySends(e, cfg),
		inactive: inactivePro(e, cfg), droppedPagers: pagersOf(dropped),
	}
}

// pagersOf keeps the paging services among names.
func pagersOf(names []string) []string {
	var pagers []string
	for _, name := range names {
		if isPagerProvider(name) {
			pagers = append(pagers, name)
		}
	}
	return pagers
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

// logDroppedPagers warns that the provider cap leaves a paging service
// inactive: incidents will not page through it, and the one provider the
// edition runs is named so the operator can change the choice.
func logDroppedPagers(n editionNotice) {
	if len(n.droppedPagers) == 0 {
		return
	}
	klog.Warningf("the Community edition runs one alert provider, so "+
		"nothing is paged through %s: component=edition sends=%q; name "+
		"the provider to keep with app.communityProvider, or get kwatch "+
		"Pro to run every provider", strings.Join(n.droppedPagers, ", "),
		n.sends)
}

// editionRecord is the value under stateEditionLast.
type editionRecord struct {
	// Edition is the edition the last start ran.
	Edition string `json:"edition"`
	// Notified is true once the move to Community was announced.
	Notified bool `json:"notified,omitempty"`
}

// editionPlan is the decision of one start: the record to keep and
// whether the operator is told. configOnly marks a notice about the
// configuration of an installation that never ran Pro: no earlier version
// and no lapsed license, only Pro settings that this edition ignores.
type editionPlan struct {
	record     editionRecord
	notify     bool
	known      bool
	configOnly bool
}

// planEdition decides from the persisted state whether this start runs
// Community after an earlier version or Pro, or runs Community with a
// configuration that sets Pro features (usesPro): the operator hears about
// that once, even on a first start, because those settings do nothing. It
// reads the state before the startup bookkeeping marks the installation
// initialized, and writes nothing: commit does, once the notice can be
// sent.
func planEdition(
	ctx context.Context, disk diskState, e feature.Entitlements,
	usesPro bool,
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
	if usesPro && !community.Notified {
		community.Notified = true
		return editionPlan{record: community, notify: true, known: true,
			configOnly: true}, nil
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
// what it does not run, what the configuration sets that is now ignored,
// and how to get it back.
func upgradeNoticeText(n editionNotice) string {
	text := notification.MarkerNotify + " kwatch is now running the " +
		"Community edition. It keeps the pod and node alerts with their " +
		"root cause; kwatch Pro restores the rest"
	if len(n.detectors) > 0 {
		text += ": " + status.ProDetectorsText(n.detectors)
	}
	text += ". " + ignoredSettingsText(n)
	return text + "Get kwatch Pro at " + upgradeTrialURL
}

// configNoticeText is the notice of an installation that never ran Pro
// but whose configuration sets Pro features: they are ignored.
func configNoticeText(n editionNotice) string {
	return notification.MarkerNotify + " kwatch is running the " +
		"Community edition. " + ignoredSettingsText(n) +
		"Get kwatch Pro at " + upgradeTrialURL
}

// ignoredSettingsText names the configured Pro features the edition
// ignores, with the sentence that ends in a space; empty when none.
func ignoredSettingsText(n editionNotice) string {
	if len(n.inactive) == 0 {
		return ""
	}
	return "Your configuration sets Pro features that are not running " +
		"and are ignored: " + strings.Join(n.inactive, "; ") + ". "
}

// announceEdition sends the edition notice, once, after an earlier
// version or Pro, or for Pro settings a Community install ignores. A fresh
// install with a Community configuration hears nothing.
func announceEdition(deps *serverDeps, disk diskState, plan editionPlan) {
	var send func(string)
	if deps.deliveryManager != nil {
		send = deps.deliveryManager.Notify
	}
	text := upgradeNoticeText(deps.notice)
	if plan.configOnly {
		text = configNoticeText(deps.notice)
	}
	plan.commit(disk, send, text)
}
