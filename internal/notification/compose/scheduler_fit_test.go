package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

func fitFacts(evidence ...detection.Evidence) caseFacts {
	return caseFacts{members: []detection.Finding{{Evidence: evidence}}}
}

func TestFitSentencesNameEachPoolAndWhatWouldFit(t *testing.T) {
	facts := fitFacts(
		pair(detection.EvidenceFit, "pool general has no node with "+
			"1.5 CPU free (best: n3 has 1.2 CPU free)"),
		pair(detection.EvidenceFit, "pool gpu has room but taint "+
			"gpu=true:NoSchedule isn't tolerated"),
		pair(detection.EvidenceFitWould, "tolerating gpu=true:NoSchedule "+
			"would fit it on gpu-1"),
		pair(detection.EvidenceFitNote, "checked 200 of 340 nodes"))
	got := fitSentences(facts)
	want := "No node fits: pool general has no node with 1.5 CPU free " +
		"(best: n3 has 1.2 CPU free); pool gpu has room but taint " +
		"gpu=true:NoSchedule isn't tolerated. Tolerating " +
		"gpu=true:NoSchedule would fit it on gpu-1. " +
		"(Note: checked 200 of 340 nodes.)"
	if len(got) != 1 || got[0].text != want || got[0].part != partProof {
		t.Errorf("got %+v, want %q", got, want)
	}
}

func TestFitSentencesSayWhenTheCheckWasSkipped(t *testing.T) {
	got := fitSentences(fitFacts(
		pair(detection.EvidenceFitNote, "the pod's scheduling rules are "+
			"too large for kwatch to check")))
	if len(got) != 1 || got[0].text != "The pod's scheduling rules are "+
		"too large for kwatch to check." {
		t.Errorf("got %+v", got)
	}
	if fitSentences(fitFacts()) != nil {
		t.Error("a finding without a verdict says nothing")
	}
}

func TestAutoscalerSentencesQuoteTheEvent(t *testing.T) {
	cases := map[string]string{
		detection.AutoscalerAdding: "The autoscaler is adding a node " +
			`for it: "pod triggered scale-up".`,
		detection.AutoscalerBlocked: "The autoscaler can't add a node " +
			`for it: "pod triggered scale-up".`,
		detection.AutoscalerLate: "The autoscaler said it was adding a " +
			`node for it, but none has taken it: "pod triggered scale-up".`,
	}
	for state, want := range cases {
		got := autoscalerSentences(fitFacts(
			pair(detection.EvidenceAutoscaler, state),
			pair(detection.EvidenceAutoscalerSays,
				"pod triggered scale-up")))
		if len(got) != 1 || got[0].text != want {
			t.Errorf("%s: got %+v, want %q", state, got, want)
		}
	}
	if autoscalerSentences(fitFacts()) != nil {
		t.Error("no autoscaler evidence means no sentence")
	}
}
