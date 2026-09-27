package config

import "testing"

func TestSuppressionIndexKeepsScopedRulesOutOfDetection(t *testing.T) {
	cfg := &Config{Silences: []SilenceRule{
		{Namespaces: []string{"dev"}, PodNamePatterns: []string{"^api-"}},
		{Reasons: []string{"OOMKilled"}, ContainerNames: []string{"worker"}},
		{PodNamePatterns: []string{"^batch-"}, LogPatterns: []string{"x"}},
		{PodNamePatterns: []string{"^canary-"}},
	}}
	index := cfg.BuildSuppressionIndex()
	if len(index.PodNamePatterns) != 1 ||
		index.PodNamePatterns[0].String() != "^canary-" {
		t.Fatalf("pod patterns = %v, want only the unscoped rule",
			index.PodNamePatterns)
	}
	if len(index.ContainerNames) != 0 || len(index.LogPatterns) != 0 {
		t.Fatalf("scoped or combined rule leaked into index: %+v", index)
	}
}
