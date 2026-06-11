package model

import (
	"testing"
)

func TestIncidentActionString(t *testing.T) {
	tests := []struct {
		action IncidentAction
		want   string
	}{
		{ActionCreate, "create"},
		{ActionUpdate, "update"},
		{ActionSkip, "skip"},
		{ActionStale, "stale"},
		{ActionResolved, "resolved"},
		{ActionDigest, "digest"},
		{ActionDigestFlush, "digest_flush"},
		{IncidentAction(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.action.String(); got != tt.want {
			t.Errorf("IncidentAction(%d).String() = %q, want %q", tt.action, got, tt.want)
		}
	}
}
