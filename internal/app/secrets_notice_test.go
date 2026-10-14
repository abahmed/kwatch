package app

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
)

func TestSecretsAccessNoticeWhenSecretsAreWatched(t *testing.T) {
	cfg := &config.Config{Watch: config.WatchConfig{Secrets: true}}

	notice := secretsAccessNotice(runtimeWith(cfg))

	for _, want := range []string{
		"every Secret", "hashes", "watch.secrets=false",
		"rbac.mode=least-privilege",
	} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice %q lacks %q", notice, want)
		}
	}
}

func TestSecretsAccessNoticeIsEmptyWhenSecretsAreOff(t *testing.T) {
	cfg := &config.Config{Watch: config.WatchConfig{Secrets: false}}

	if notice := secretsAccessNotice(runtimeWith(cfg)); notice != "" {
		t.Fatalf("notice = %q, want none", notice)
	}
}
