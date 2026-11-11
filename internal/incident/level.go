package incident

import (
	"strings"

	"github.com/abahmed/kwatch/internal/format"
)

// Level codes say why an incident is at its tier. They are persisted in
// the journal and turned into plain words by AlertLevelText, so a code
// is never shown as it is.
const (
	levelPagePrefix = "page:"
	levelPageHeld   = "page-held"
	levelNotify     = "notify"
	levelDigestOnly = "digest-only"
	levelRoutine    = "routine"
	levelKnown      = "known"
	levelRhythm     = "rhythm"
	levelCauseOnly  = "cause-only"
	levelAdvisory   = "advisory"
	levelDrain      = "drain"
	levelIdle       = "idle-webhook"
	levelKept       = "kept"
	levelLowered    = "lowered"
	levelLoweredTo  = "lowered-notify"
	levelSilenced   = "lowered-silent"
	levelOverride   = "override"
	levelNoPaging   = "paging-off"
	levelNonProd    = "non-production"
	// levelOwnStart is in own_start.go.
)

// fixedLevel is the code of a tier fixedTier sets.
func fixedLevel(p *Incident) string {
	switch {
	case len(p.Members) == 0:
		return ""
	case drainingRoot(p):
		return levelDrain
	}
	return levelIdle
}

// levelOf is why p is at its tier now: the policy's reason, unless the
// configured overrides, the ratchet or a lowering moved the tier.
func (m *Manager) levelOf(p *Incident) string {
	base, why := tierWhy(p)
	return m.levelFor(p, base, why, m.override.apply(p, base))
}

// levelFor names why p.Tier differs from base, the policy's tier with
// its code why, and overridden, that tier after the overrides.
func (m *Manager) levelFor(
	p *Incident, base Tier, why string, overridden Tier,
) string {
	switch {
	case p.Tier > overridden:
		return levelKept
	case p.Tier < overridden:
		return loweredLevel(p.Tier)
	case overridden < base && base == Page && m.override.nonProd:
		return levelNonProd
	case overridden < base && base == Page && m.override.noPage:
		return levelNoPaging
	case overridden != base:
		return levelOverride
	}
	return why
}

// loweredLevel is the code of a tier lowered to tier: the words say
// where it went.
func loweredLevel(tier Tier) string {
	switch {
	case tier >= Notify:
		return levelLoweredTo
	case tier == Silent:
		return levelSilenced
	}
	return levelLowered
}

// noteLevel stores why p is at its tier, keeping the earlier code when
// the policy has none (an incident without members).
func (p *Incident) noteLevel(code string) {
	if code != "" {
		p.Journal.Level = code
	}
}

// levelTexts are the plain words of each level code.
var levelTexts = map[string]string{
	levelPageHeld: "Needs attention instead of Urgent: the same " +
		"failure was Urgent less than " + format.Duration(RepageWindow) +
		" ago.",
	levelNotify: "Needs attention: something failed, but users are " +
		"still served and the cluster keeps working.",
	levelDigestOnly: "Low: kwatch lists it in the daily summary and " +
		"sends no alert. It holds only informational findings or " +
		"planned disruption.",
	levelRoutine: "Low: kwatch lists it in the daily summary and " +
		"sends no alert. It happens at about the same time every day " +
		"and ends by itself.",
	levelKnown: "Low: kwatch lists it in the daily summary and sends " +
		"no alert. People have been alerted about it for a day " +
		"already.",
	levelRhythm: "Low: kwatch lists it in the daily summary and sends " +
		"no alert. It fails and recovers on a regular schedule.",
	levelCauseOnly: "No alert of its own: it is the cause of other " +
		"alerts, and those alerts name it.",
	levelAdvisory: "No alert: throttling or a full quota that has " +
		"not broken anything yet.",
	levelDrain: "Planned maintenance: a node is being drained on " +
		"purpose, so its pods are expected to move.",
	levelIdle: "Low: kwatch lists it in the daily summary and sends " +
		"no alert. The failing webhook receives no requests right now.",
	levelKept: "Kept at its highest level: it was worse earlier, and " +
		"stays at that level until it resolves.",
	levelLowered: "Lowered to Low: the failures that made it louder " +
		"have cleared.",
	levelLoweredTo: "Lowered to Needs attention: the failures that " +
		"made it Urgent have cleared.",
	levelSilenced: "Lowered to no alert: the failures left need no " +
		"one's attention.",
	levelOverride: "Set by a severity override in kwatch's " +
		"configuration.",
	levelNoPaging: "Needs attention instead of Urgent: paging is " +
		"turned off in kwatch's configuration.",
	levelNonProd: NonProductionText + ". Needs attention instead of " +
		"Urgent: kwatch's configuration marks this cluster as not " +
		"production.",
	levelOwnStart: "No alert: kwatch's own pod waits for its volume " +
		"while kwatch restarts, which every restart does for a while.",
}

// pageTexts are the plain words of each page rule.
var pageTexts = map[string]string{
	"cluster-dns-failing": "cluster DNS is failing.",
	"api-unavailable":     "the Kubernetes API is unavailable.",
	"scheduler-down":      "the scheduler is down; no new pod starts.",
	"controller-manager-down": "the controller manager is down; " +
		"nothing is reconciled.",
	"admission-blocked": "a fail-closed admission webhook blocks " +
		"creates.",
	"traffic-lost": "users lose traffic: a route has no ready " +
		"backend.",
	"exposed-service-lost": "an exposed Service has no ready backend.",
	"last-replica-down":    "a user-facing workload has no ready replica.",
	"node-lost":            "a node stopped working.",
}

// AlertLevelText says in plain words why an incident is at its tier,
// from the code its journal recorded; "" for none or an unknown code.
func AlertLevelText(code string) string {
	if rule, ok := strings.CutPrefix(code, levelPagePrefix); ok {
		if text, known := pageTexts[rule]; known {
			return "Urgent: " + text
		}
		return "Urgent: it matches a rule for failures that need " +
			"someone now."
	}
	if text, ok := chronicText(code); ok {
		return text
	}
	return levelTexts[code]
}

// keepLevel makes sure a closing incident keeps a level code, so a
// resolved incident still says why it was at its tier: the policy's
// code while members are left, else the plain code of its tier.
func (m *Manager) keepLevel(p *Incident) {
	if p.Journal.Level != "" {
		return
	}
	p.noteLevel(m.levelOf(p))
	if p.Journal.Level != "" {
		return
	}
	switch p.Tier {
	case Page, Notify:
		p.noteLevel(levelNotify)
	case Digest:
		p.noteLevel(levelDigestOnly)
	}
}
