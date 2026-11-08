package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func blastCause(kind inventory.Kind, count, total int, supports bool,
) caseFacts {
	return caseFacts{p: incident.Incident{Cause: &rootcause.CauseRecord{
		Root: inventory.CoreID(kind, "shop", "orders-config"),
		Proof: []rootcause.Proof{{Code: rootcause.ProofBlastOverlap,
			Count: count, Total: total, Supports: supports}}}}}
}

func TestBlastSentenceGivesTheShareByTheKindsVerb(t *testing.T) {
	cm := blastSentences(blastCause(kube.KindConfigMap, 9, 10, true))
	if assert.Len(t, cm, 1) {
		assert.Equal(t, "9 of 10 failing pods read it.", cm[0].text)
	}
	secret := blastSentences(blastCause(kube.KindSecret, 3, 4, true))
	assert.Equal(t, "3 of 4 failing pods use it.", secret[0].text)
	claim := blastSentences(blastCause(kube.KindPVC, 2, 2, true))
	assert.Equal(t, "2 of 2 failing pods mount it.", claim[0].text)
}

func TestBlastSentenceIsToldOnlyForAMajorityOfSeveralPods(t *testing.T) {
	assert.Empty(t, blastSentences(blastCause(kube.KindConfigMap, 4, 10, true)))
	assert.Empty(t, blastSentences(blastCause(kube.KindConfigMap, 1, 1, true)))
	assert.Empty(t, blastSentences(blastCause(kube.KindConfigMap, 9, 10, false)))
	assert.Empty(t, blastSentences(caseFacts{}))
	assert.Empty(t, blastSentences(blastCause(kube.KindDeployment, 5, 5, true)),
		"only objects pods read, use or mount")
	hedge := blastCause(kube.KindConfigMap, 5, 5, true)
	hedge.p.Cause.Rival = &rootcause.CauseRecord{}
	assert.Empty(t, blastSentences(hedge), "a hedge between two causes")
}
