package app

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/feature"
)

func TestSecretsAccessNoticeWhenSecretsAreWatched(t *testing.T) {
	cfg := &config.Config{Watch: config.WatchConfig{Secrets: true}}

	notice := secretsAccessNotice(runtimeWith(cfg), feature.ProEntitlements())

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

	notice := secretsAccessNotice(runtimeWith(cfg), feature.ProEntitlements())
	if notice != "" {
		t.Fatalf("notice = %q, want none", notice)
	}
}

// Community runs no detector on Secrets, yet keeps the Secret informer for
// the root-cause engine; the notice says so, and Pro's notice does not.
func TestSecretsAccessNoticeSaysWhatCommunityUsesSecretsFor(t *testing.T) {
	cfg := &config.Config{Watch: config.WatchConfig{Secrets: true}}

	community := secretsAccessNotice(
		runtimeWith(cfg), feature.CommunityEntitlements())
	pro := secretsAccessNotice(runtimeWith(cfg), feature.ProEntitlements())

	if !strings.Contains(community, "a Secret a pod references is missing") {
		t.Errorf("community notice = %q", community)
	}
	if strings.Contains(pro, "Community edition") {
		t.Errorf("pro notice = %q", pro)
	}
	if !strings.Contains(community, "watch.secrets=false") {
		t.Errorf("community notice lacks the way out: %q", community)
	}
}
