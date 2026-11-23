package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// errorSentences quote the failure's own error, else the first error
// line investigation found, else the last line the application wrote.
func errorSentences(f caseFacts) []sentence {
	if !f.ok || hasProof(nodeWhySentences(f)) {
		return nil
	}
	said := evidence(f.lead, "error", "message")
	if kube.IsRuntimeLogFailure(said) {
		// The kubelet failing to read the log is not the app's error.
		said = ""
	}
	switch {
	case pullNoise.MatchString(said):
		if !strings.Contains(leadText(f), " pull") {
			return []sentence{{part: partProof, weight: weightError,
				text: "Its image cannot be pulled."}}
		}
	case said != "" && (quotedByMissingService(f, said) ||
		quotedByStartLead(f, said) || quotedByCustomDetail(f, said) ||
		quotedByProbeLead(f, said)):
		return nil
	case said != "" && !restartNoise.MatchString(said):
		return []sentence{{part: partProof, weight: weightError,
			text: errorIntro(f) + " " + quoted(said) + "."}}
	}
	if first := investigatedError(f); first != nil {
		return first
	}
	if last := lastLine(f.output); last != "" {
		return []sentence{{part: partProof, weight: weightError,
			text: "Its last output was " + quoted(last) + "."}}
	}
	return nil
}

// usageSentences state how full something is: "It is at 94% of 20 GiB."
// or "etl uses about 11 GiB."
func usageSentences(f caseFacts) []sentence {
	var out []sentence
	for _, s := range f.members {
		used := evidence(s, "used")
		if used == "" {
			continue
		}
		subject := "It"
		if s.Entity != f.lead.Entity {
			subject = nameFrom(f.p.Root, s.Entity)
		}
		text := subject + " uses " + humanBytes(used)
		if strings.HasSuffix(used, "%") {
			text = subject + " is at " + usedOfWords(used, s)
		}
		out = append(out, sentence{part: partProof, weight: weightUsage,
			text: endSentence(text + volumeUsersTail(s))})
	}
	return out
}

// schedulerSentences say why the scheduler rejects every node: the
// investigated node counts, else the scheduler's own words.
func schedulerSentences(f caseFacts) []sentence {
	if counted := investigatedScheduler(f); counted != nil {
		return counted
	}
	out := schedulerNumbers(f)
	for _, s := range f.members {
		if said := evidence(s, "scheduler"); said != "" {
			return append(out, sentence{part: partProof,
				weight: weightScheduler,
				text:   "The scheduler says " + quoted(said) + "."})
		}
	}
	return out
}

// reasonConsequences say what the lead's own condition breaks.
var reasonConsequences = map[string]string{
	reasons.TLSCertExpired:         "Clients are rejecting it now.",
	reasons.WebhookNoEndpoints:     webhookRejected,
	reasons.WebhookBackendNotFound: webhookRejected,
}

// webhookRejected says what a webhook with a dead backend did: it
// fails closed, and the API server rejected requests because of it.
const webhookRejected = "It fails closed (failurePolicy Fail) and the " +
	"API server has rejected requests because of it."

// idleWebhookText says what a fail-closed webhook nothing was refused
// by yet does. When its rules cover objects that exist now, their
// creates and updates are refused: the lead says so when it is the
// webhook's own summary, else this sentence does.
func idleWebhookText(f caseFacts) string {
	names := evidence(f.lead, detection.EvidenceWebhookIntercepts)
	switch {
	case names != "" && f.p.Root == f.lead.Entity:
		return ""
	case names != "":
		return "It fails closed (failurePolicy Fail) and covers " +
			names + " that exist now: their creates and updates are " +
			"rejected right now."
	}
	return "It fails closed (failurePolicy Fail); no request has been " +
		"rejected so far."
}

// consequenceSentences say what the lead's condition breaks. A webhook
// blocks requests only when its failurePolicy is Fail: critical once a
// request was refused, a warning before, and then it says so.
// An expired certificate only has clients rejecting it when something
// uses it, which is also what makes it critical; one nothing references
// is stated as a fact and breaks nothing.
func consequenceSentences(f caseFacts) []sentence {
	text, ok := reasonConsequences[f.lead.Reason]
	if !f.ok || !ok {
		return nil
	}
	webhook := f.lead.Reason == reasons.WebhookNoEndpoints ||
		f.lead.Reason == reasons.WebhookBackendNotFound
	expired := f.lead.Reason == reasons.TLSCertExpired
	if s, ok := conversionConsequence(f, webhook); ok {
		return s
	}
	if webhook && f.lead.Severity == detection.Warning {
		text := idleWebhookText(f)
		if text == "" {
			return nil
		}
		return []sentence{{part: partConsequence, weight: 1, text: text}}
	}
	if (webhook || expired) && f.lead.Severity != detection.Critical {
		return nil
	}
	return []sentence{{part: partConsequence, weight: 1, text: text}}
}
