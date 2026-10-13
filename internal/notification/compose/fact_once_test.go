package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A fact is said once: the plain sentence goes when a joined one carries
// it, and stays when none does.
func TestArrangeSaysAFactOnce(t *testing.T) {
	plain := sentence{part: partProof, weight: 1, text: "It used 5Mi.",
		fact: factMemory}
	other := sentence{part: partProof, weight: 0.5, text: "Other."}
	joined := sentence{part: partConsequence, weight: 1,
		text:    "It used 5Mi; it failed after a release.",
		carries: []factKey{factMemory}}

	alone := arrange([]sentence{plain, other})
	assert.Equal(t, []string{"It used 5Mi.", "Other."}, texts(alone))

	both := arrange([]sentence{plain, other, joined})
	assert.Equal(t, []string{"Other.",
		"It used 5Mi; it failed after a release."}, texts(both))
}

// A dropped sentence does not use up a place in the proof.
func TestCarriedSentenceDoesNotCountAsProof(t *testing.T) {
	var in []sentence
	in = append(in, sentence{part: partProof, weight: 3, text: "a",
		fact: factLiveness})
	in = append(in, sentence{part: partProof, weight: 2, text: "b"})
	in = append(in, sentence{part: partProof, weight: 1, text: "c"})
	in = append(in, sentence{part: partConsequence, text: "d",
		carries: []factKey{factLiveness}})
	assert.Equal(t, []string{"b", "c", "d"}, texts(arrange(in)))
}

func texts(sentences []sentence) []string {
	var out []string
	for _, s := range sentences {
		out = append(out, s.text)
	}
	return out
}
