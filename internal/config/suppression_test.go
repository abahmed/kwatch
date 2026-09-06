package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildSuppressionIndexIncludesEventMessages(t *testing.T) {
	cfg := &Config{
		Silences: []SilenceRule{{
			EventMessages: []string{
				"sync configmap cache",
				"failed to mount",
				"sync configmap cache",
			},
		}},
	}

	index := cfg.BuildSuppressionIndex()

	assert.Equal(
		t,
		[]string{"sync configmap cache", "failed to mount"},
		index.EventMessages,
	)
}
