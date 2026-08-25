package v1alpha1

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSilenceRuleDeepCopyIsNotAliased(t *testing.T) {
	in := &SilenceRule{
		Namespaces:        []string{"ns1"},
		Reasons:           []string{"OOMKilled"},
		PodNamePatterns:   []string{"^app-"},
		ContainerNames:    []string{"sidecar"},
		LogPatterns:       []string{"fatal"},
		ContainerMessages: []string{"denied"},
		NodeReasons:       []string{"KubeletNotReady"},
		NodeMessages:      []string{"draining"},
	}

	out := in.DeepCopy()

	// Mutating the copy must not affect the original (no aliased slices).
	out.Namespaces[0] = "mutated"
	out.Reasons[0] = "mutated"
	out.PodNamePatterns[0] = "mutated"
	out.ContainerNames[0] = "mutated"
	out.LogPatterns[0] = "mutated"
	out.ContainerMessages[0] = "mutated"
	out.NodeReasons[0] = "mutated"
	out.NodeMessages[0] = "mutated"

	assert.Equal(t, []string{"ns1"}, in.Namespaces)
	assert.Equal(t, []string{"OOMKilled"}, in.Reasons)
	assert.Equal(t, []string{"^app-"}, in.PodNamePatterns)
	assert.Equal(t, []string{"sidecar"}, in.ContainerNames)
	assert.Equal(t, []string{"fatal"}, in.LogPatterns)
	assert.Equal(t, []string{"denied"}, in.ContainerMessages)
	assert.Equal(t, []string{"KubeletNotReady"}, in.NodeReasons)
	assert.Equal(t, []string{"draining"}, in.NodeMessages)
}
