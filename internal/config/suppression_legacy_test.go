package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIgnoreFieldsBecomeSilences(t *testing.T) {
	cfg := &Config{
		Silences:                []SilenceRule{{Reasons: []string{"Evicted"}}},
		IgnoreContainerNames:    []string{"sidecar"},
		IgnorePodNames:          []string{"^batch-"},
		IgnoreContainerMessages: []string{"back-off"},
		IgnoreNodeReasons:       []string{"NodeNotReady"},
		IgnoreNodeMessages:      []string{"kubelet stopped"},
	}

	assert.Equal(t, []SilenceRule{
		{Reasons: []string{"Evicted"}},
		{ContainerNames: []string{"sidecar"}},
		{PodNamePatterns: []string{"^batch-"}},
		{ContainerMessages: []string{"back-off"}},
		{NodeReasons: []string{"NodeNotReady"}},
		{NodeMessages: []string{"kubelet stopped"}},
	}, appendIgnoreFieldSilences(cfg))
}

func TestPrepareConfigDoesNotDuplicateSyntheticSilences(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IgnoreContainerNames = []string{"sidecar"}

	prepareConfig(cfg)
	prepareConfig(cfg)

	assert.Len(t, cfg.Silences, 1)
}
