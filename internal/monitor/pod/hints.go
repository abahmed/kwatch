package pod

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

type imagePullPattern struct {
	match           []string
	hint            string
	hintWithSecrets string
}

var imagePullPatterns = []imagePullPattern{
	{
		match: []string{"toomanyrequests", "rate limit"},
		hint: "Docker Hub rate limit exceeded — add imagePullSecrets for " +
			"authenticated pulls or configure a mirror registry",
	},
	{
		match: []string{"pull qps"},
		hint: "Kubelet image pull QPS limit exceeded — consider increasing " +
			"registryPullQPS in kubelet config or reducing concurrent pod starts",
	},
	{
		match: []string{
			"authentication required",
			"unauthorized",
			"denied",
			"no pull access",
		},
		hint: "Registry authentication failed — check imagePullSecrets validity",
	},
	{
		match: []string{
			"not found",
			"manifest unknown",
			"does not exist",
		},
		hint: "Image not found — check the image name/tag",
		hintWithSecrets: "Image not found — check the image name/tag, or the " +
			"image may not exist in this registry",
	},
	{
		match: []string{"context deadline exceeded", "i/o timeout"},
		hint: "Registry connection timed out — check network connectivity to " +
			"the registry and DNS resolution",
	},
	{
		match: []string{"connection refused", "connection reset"},
		hint: "Registry connection refused — check that the registry is running " +
			"and not blocked by a firewall",
	},
	{
		match: []string{"no route to host", "network is unreachable"},
		hint: "No network route to registry — check firewall rules and network " +
			"connectivity",
	},
	{
		match: []string{"no such host", "dial tcp"},
		hint:  "Registry unreachable — check cluster network connectivity and DNS",
	},
	{
		match: []string{"tls", "certificate"},
		hint: "Registry TLS error — check registry certificate or configure " +
			"insecure-registries",
	},
}

// ImagePullMessageHint returns targeted guidance for a kubelet image-pull
// message. It returns an empty string when no known pattern matches.
func ImagePullMessageHint(msg string, hasSecrets bool) string {
	msg = strings.ToLower(msg)
	for _, pattern := range imagePullPatterns {
		if !containsAny(msg, pattern.match) {
			continue
		}
		if hasSecrets && pattern.hintWithSecrets != "" {
			return pattern.hintWithSecrets
		}
		return pattern.hint
	}
	return ""
}

func containsAny(msg string, patterns []string) bool {
	for _, pattern := range patterns {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

// NeedsRegistryAuth reports whether an image uses an explicit registry host
// where credentials are commonly required.
func NeedsRegistryAuth(image string) bool {
	slash := strings.IndexByte(image, '/')
	if slash < 0 {
		return false
	}
	host := image[:slash]
	return strings.Contains(host, ".") || strings.Contains(host, ":")
}

// BuildProbeHint explains the configured probe that failed.
func BuildProbeHint(reason string, spec *corev1.Container) string {
	probe := probeForReason(reason, spec)
	if probe == nil {
		return enricher.HintForReason(reason)
	}

	detail := reason
	if endpoint := ProbeEndpoint(reason, spec); endpoint != "" {
		detail = reason + " (" + endpoint + ")"
	}
	return fmt.Sprintf(
		"%s — application not responding to %s probe",
		detail,
		ProbeType(reason),
	)
}

// ProbeEndpoint describes what a configured probe checks.
func ProbeEndpoint(reason string, spec *corev1.Container) string {
	probe := probeForReason(reason, spec)
	if probe == nil {
		return ""
	}
	switch {
	case probe.HTTPGet != nil:
		host := probe.HTTPGet.Host
		if host == "" {
			host = spec.Name
		}
		scheme := "http"
		if probe.HTTPGet.Scheme == corev1.URISchemeHTTPS {
			scheme = "https"
		}
		return fmt.Sprintf(
			"HTTP GET %s://%s%s%s",
			scheme,
			host,
			probePort(probe.HTTPGet.Port),
			probe.HTTPGet.Path,
		)
	case probe.TCPSocket != nil:
		return "TCP check " + probePort(probe.TCPSocket.Port)
	case probe.Exec != nil:
		if len(probe.Exec.Command) == 0 {
			return "exec "
		}
		return "exec " + probe.Exec.Command[0]
	default:
		return ""
	}
}

// ProbeType returns the user-facing name for a probe reason.
func ProbeType(reason string) string {
	switch reason {
	case constant.ReasonLivenessProbeFailed:
		return "liveness"
	case constant.ReasonReadinessProbeFailed:
		return "readiness"
	case constant.ReasonStartupProbeFailed:
		return "startup"
	default:
		return "probe"
	}
}

func probeForReason(
	reason string,
	spec *corev1.Container,
) *corev1.Probe {
	if spec == nil {
		return nil
	}
	switch reason {
	case constant.ReasonLivenessProbeFailed:
		return spec.LivenessProbe
	case constant.ReasonReadinessProbeFailed:
		return spec.ReadinessProbe
	case constant.ReasonStartupProbeFailed:
		return spec.StartupProbe
	default:
		return nil
	}
}

func probePort(port intstr.IntOrString) string {
	if port.Type == intstr.String {
		if port.StrVal == "" {
			return ""
		}
		return ":" + port.StrVal
	}
	if port.IntValue() == 0 {
		return ""
	}
	return ":" + strconv.Itoa(port.IntValue())
}
