package incident

import (
	"testing"

	"github.com/abahmed/kwatch/internal/model"
)

func TestGlobalImagePullIncidentIsNotResolvedByOnePod(t *testing.T) {
	e := newTestEngine(Config{})
	inc := &model.Incident{
		Subject: model.Subject{
			Key:      GlobalKey("ImagePullBackOff", "rate_limit"),
			Reason:   "ImagePullBackOff",
			Resource: "pod", Namespace: "a", ContainerName: "app",
		},
		Status: model.Status{
			State:     model.StateActive,
			Resources: map[string]bool{"p1": true, "p2": true},
		},
	}
	if e.podRecoveryResolves(inc, "p1", map[string]bool{"app": true}) {
		t.Fatal("one namespace's recovery resolved a global incident")
	}
}
