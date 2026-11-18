package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

func youngMember(value string) detection.Finding {
	return detection.Finding{Evidence: []detection.Evidence{{
		Label: detection.EvidenceYoungNode, Value: value}}}
}

func TestYoungNodeSentencesSayTheNodeAge(t *testing.T) {
	got := youngNodeSentences(caseFacts{members: []detection.Finding{
		youngMember("n3 2"), youngMember("n3 4")}})
	want := "The failing pods run on node n3, which joined 2 minutes " +
		"before they began failing."
	if len(got) != 1 || got[0].text != want {
		t.Fatalf("got %+v", got)
	}
	two := youngNodeSentences(caseFacts{members: []detection.Finding{
		youngMember("n4 0"), youngMember("n3 1")}})
	want = "The failing pods run on nodes n3 and n4, which joined under " +
		"a minute before they began failing."
	if len(two) != 1 || two[0].text != want {
		t.Fatalf("got %+v", two)
	}
}

func TestYoungNodeSentencesNeedTheFact(t *testing.T) {
	if got := youngNodeSentences(caseFacts{members: []detection.Finding{
		{}, youngMember("garbage")}}); got != nil {
		t.Fatalf("got %+v", got)
	}
}
