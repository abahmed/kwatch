package pod

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	"github.com/abahmed/kwatch/internal/constant"
)

func TestImagePullMessageHint(t *testing.T) {
	tests := []struct {
		name        string
		message     string
		secrets     bool
		wantContain string
	}{
		{
			name:        "rate limit",
			message:     "toomanyrequests: pull limit",
			wantContain: "rate limit",
		},
		{
			name:        "authentication",
			message:     "unauthorized: access denied",
			wantContain: "authentication",
		},
		{
			name:        "not found with credentials",
			message:     "manifest unknown",
			secrets:     true,
			wantContain: "may not exist",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ImagePullMessageHint(test.message, test.secrets)
			if test.wantContain == "" {
				if got != "" {
					t.Fatalf("hint = %q, want empty", got)
				}
				return
			}
			if !containsAny(got, []string{test.wantContain}) {
				t.Fatalf("hint = %q, want %q", got, test.wantContain)
			}
		})
	}
}

func TestProbeEndpoint(t *testing.T) {
	spec := &corev1.Container{
		Name: "api",
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
				Path:   "/ready",
				Port:   intstr.FromString("http"),
				Scheme: corev1.URISchemeHTTPS,
			}},
		},
	}

	if got := ProbeEndpoint(
		constant.ReasonReadinessProbeFailed,
		spec,
	); got != "HTTP GET https://api:http/ready" {
		t.Fatalf("endpoint = %q", got)
	}
}
