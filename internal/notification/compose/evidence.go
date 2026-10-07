package compose

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Proof weights of investigated facts. A changed config key is almost
// as convincing as the change itself; a node's heaviest pods only
// explain where the pressure comes from.
const (
	weightKeys        = 0.7
	weightEndpoints   = 0.6
	weightNode        = 0.5
	weightPull        = 0.5
	weightWebhook     = 0.5
	weightTopMemory   = 0.45
	weightTopCPU      = 0.4
	weightTermination = 0.3
)

// pullWords say what a pull error class means.
var pullWords = map[string]string{
	"image":      "the image or tag does not exist",
	"rate-limit": "the registry is rate limiting pulls",
	"auth":       "the registry rejects the credentials",
	"server":     "the registry answers with server errors",
	"tls":        "the registry's certificate is not trusted",
	"network":    "the registry cannot be reached",
}

// evidenceWriters turn one investigated fact into a sentence. The error
// line and scheduler counts are written by errorSentences and
// schedulerSentences, next to the facts they replace.
var evidenceWriters = map[string]func(incident.Fact) (sentence, bool){
	incident.FactTermination: func(e incident.Fact) (sentence, bool) {
		return proof(weightTermination,
			"Its last run ended with "+quoted(e.Text)+"."), true
	},
	incident.FactNode: func(e incident.Fact) (sentence, bool) {
		return proof(weightNode, "The node reports "+e.Text+"."), true
	},
	incident.FactTopMemory: func(e incident.Fact) (sentence, bool) {
		return proof(weightTopMemory,
			"The pods using the most memory there are "+e.Text+"."), true
	},
	incident.FactTopCPU: func(e incident.Fact) (sentence, bool) {
		return proof(weightTopCPU,
			"The pods using the most CPU there are "+e.Text+"."), true
	},
	incident.FactKeys: func(e incident.Fact) (sentence, bool) {
		return proof(weightKeys, "The change touched keys "+e.Text+
			" of "+e.Subject+"."), true
	},
	incident.FactEndpoints: endpointSentence,
	incident.FactDependency: func(e incident.Fact) (sentence, bool) {
		return proof(weightEndpoints,
			"Its output names "+e.Text+"."), true
	},
	incident.FactWebhook: func(e incident.Fact) (sentence, bool) {
		return proof(weightWebhook,
			"The API server says "+quoted(e.Text)+"."), true
	},
	incident.FactPull: func(e incident.Fact) (sentence, bool) {
		words, ok := pullWords[e.Text]
		return proof(weightPull, "The pull fails because "+words+"."), ok
	},
}

func endpointSentence(e incident.Fact) (sentence, bool) {
	count := "no ready endpoints"
	if e.Text != "0" {
		count = e.Text + " ready endpoints"
		if e.Text == "1" {
			count = "1 ready endpoint"
		}
	}
	return proof(weightEndpoints, "Service "+e.Subject+
		" behind the webhook has "+count+"."), true
}

func proof(weight float64, text string) sentence {
	return sentence{part: partProof, weight: weight, text: text}
}

// evidenceSentences write the facts investigation found.
func evidenceSentences(f caseFacts) []sentence {
	var out []sentence
	for _, e := range f.evidence {
		if write, ok := evidenceWriters[e.Kind]; ok {
			if s, ok := write(e); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

// investigated is the text of the first investigated fact of kind fact.
func investigated(f caseFacts, fact string) string {
	for _, e := range f.evidence {
		if e.Kind == fact && e.Text != "" &&
			!kube.IsRuntimeLogFailure(e.Text) {
			return e.Text
		}
	}
	return ""
}

// freshEvidenceSentences add to an update the strongest fact that
// investigation found after the announcement. The pipeline passes only
// facts nobody has heard yet.
func freshEvidenceSentences(f caseFacts) []sentence {
	if len(f.evidence) == 0 {
		return nil
	}
	out := append(investigatedError(f), investigatedScheduler(f)...)
	out = append(out, evidenceSentences(f)...)
	return limitSentences(arrange(out), 1)
}

// investigatedError quotes the first meaningful error line a crashed
// container wrote.
func investigatedError(f caseFacts) []sentence {
	text := investigated(f, incident.FactError)
	if text == "" {
		return nil
	}
	return []sentence{proof(weightError,
		"It fails with "+quoted(text)+".")}
}

// investigatedScheduler counts the nodes each scheduling blocker
// rejects.
func investigatedScheduler(f caseFacts) []sentence {
	text := investigated(f, incident.FactScheduler)
	if text == "" {
		return nil
	}
	return []sentence{proof(weightScheduler,
		"The scheduler rejects every node: "+text+".")}
}
