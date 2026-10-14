package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

// TestClassifyPullIgnoresStatusLikeTags: a tag such as "1.503" or
// "429" is not an HTTP status; only a code the registry answered with
// classifies the pull.
func TestClassifyPullIgnoresStatusLikeTags(t *testing.T) {
	cases := map[string]detection.Mode{
		`Failed to pull image "registry.example.com/app:1.503": rpc ` +
			`error: code = Unknown desc = unexpected EOF`: pullImage,
		`Back-off pulling image "r.example/app:429"`:    pullImage,
		`Failed to pull image "r.example/app:v401-403"`: pullImage,
		`Failed to pull image "registry.example.com/app:1.503": ` +
			`unexpected status code 503 Service Unavailable`: pullServer,
		`pulling "app:2": received unexpected HTTP status: ` +
			`500 Internal Server Error`: pullServer,
		`failed to fetch anonymous token: ` +
			`unexpected status: 401`: pullAuth,
		`GET https://r.example/v2/: HTTP/1.1 429`: pullRateLimit,
		`failed to authorize: 403`:                pullAuth,
	}
	for text, want := range cases {
		if got := classifyPull(text); got != want {
			t.Errorf("classifyPull(%q) = %q, want %q", text, got, want)
		}
	}
}

// TestNodeAgentSignalNeedsKubeletPhrasing: only the kubelet and runtime
// wordings for a network plugin or storage driver that is not ready
// match; a name that merely contains "cni" or "csi driver" does not.
func TestNodeAgentSignalNeedsKubeletPhrasing(t *testing.T) {
	match := []string{
		"network plugin is not ready: cni config uninitialized",
		"NetworkPluginNotReady",
		"cni plugin not initialized",
		"failed to setup network for sandbox \"abc\": plugin type",
		"Failed to set up sandbox container \"abc\" network for pod",
		"driver name ebs.csi.aws.com not found in the list of " +
			"registered CSI drivers",
		"CSINode node-1 does not contain driver ebs.csi.aws.com",
	}
	for _, text := range match {
		if !hasSignal(SignalNodeAgent, text) {
			t.Errorf("%q must match", text)
		}
	}
	noMatch := []string{
		`Back-off pulling image "calico/cni:v3.26"`,
		"lookup cni.internal.example.com: no such host",
		"my.cni.value", "cni-1 ok",
		"app: csi driver sync timed out; retry",
		"cannot connect to the network plugin registry of vendor X",
	}
	for _, text := range noMatch {
		if hasSignal(SignalNodeAgent, text) {
			t.Errorf("%q must not match", text)
		}
	}
}

// TestNodeAgentSignalIgnoresKwatchSummary: the signal reads the quoted
// error text, never a finding summary written by kwatch.
func TestNodeAgentSignalIgnoresKwatchSummary(t *testing.T) {
	f := newFixture(t)
	agent, nodes := agentOnN1(f)
	pods := f.workload("shop", "api", 1, nodes[0])
	f.workload("shop", "web", 1, nodes[0])
	f.fail(pods[0], detection.ModeCreating, failingH, 2, "")
	f.findings[pods[0]][0].Summary = "network plugin is not ready"

	if blamesAgent(f, pods[0], agent) {
		t.Fatal("a summary must not stand in for the error text")
	}
}
