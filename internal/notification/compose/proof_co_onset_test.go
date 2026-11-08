package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/rootcause"
)

func TestCoOnsetProofStatesTheGapOnly(t *testing.T) {
	two := rootcause.Proof{Code: rootcause.ProofCoOnset, Count: 2, Total: 4}
	assert.Equal(t, "they started failing 4 seconds apart", proofWords(two))
	one := rootcause.Proof{Code: rootcause.ProofCoOnset, Count: 2, Total: 1}
	assert.Equal(t, "they started failing 1 second apart", proofWords(one))
	many := rootcause.Proof{Code: rootcause.ProofCoOnset, Count: 5, Total: 12}
	assert.Equal(t, "all started failing within 12 seconds",
		proofWords(many))
	same := rootcause.Proof{Code: rootcause.ProofCoOnset, Count: 2}
	assert.Equal(t, "they started failing at the same time",
		proofWords(same))
}
