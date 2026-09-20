package filter

import (
	"regexp"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
)

func TestMatchesSuppressionRules(t *testing.T) {
	index := config.SuppressionIndex{
		ContainerNames:    []string{"sidecar"},
		PodNamePatterns:   []*regexp.Regexp{regexp.MustCompile(`^api-`)},
		LogPatterns:       []*regexp.Regexp{regexp.MustCompile(`permission`)},
		ContainerMessages: []string{"failed to mount"},
		EventMessages:     []string{"quota exceeded"},
		NodeReasons:       []string{"KubeletNotReady"},
		NodeMessages:      []string{"disk pressure"},
	}
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"container name", MatchesContainerName(index, "sidecar"), true},
		{"pod pattern", MatchesPodName(index, "api-0"), true},
		{"log pattern", MatchesLog(index, "permission denied"), true},
		{
			"container message",
			MatchesContainerMessage(index, "failed to mount volume"), true,
		},
		{
			"event message",
			MatchesEventMessage(index, "quota exceeded for namespace"), true,
		},
		{"node reason", MatchesNodeReason(index, "KubeletNotReady"), true},
		{"node message", MatchesNodeMessage(index, "disk pressure is high"), true},
		{"empty event", MatchesEventMessage(index, ""), false},
		{"different value", MatchesPodName(index, "worker-0"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("got %t, want %t", tc.got, tc.want)
			}
		})
	}
}
