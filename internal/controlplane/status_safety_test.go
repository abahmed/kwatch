package controlplane

import (
	"context"
	"errors"
	"testing"
)

func TestSafeProbeErrorDoesNotExposeTransportDetails(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want string
	}{
		{
			name: "timeout",
			err:  context.DeadlineExceeded,
			want: "timeout",
		},
		{
			name: "dns",
			err:  errors.New("DNS lookup secret-token.example: no such host"),
			want: "dns_failed",
		},
		{
			name: "generic",
			err:  errors.New("Authorization: Bearer secret-token"),
			want: "probe_failed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := safeProbeError(test.err); got != test.want {
				t.Fatalf("safeProbeError() = %q, want %q", got, test.want)
			}
		})
	}
}
