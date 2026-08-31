package correlation

import (
	"testing"

	"github.com/abahmed/kwatch/internal/event"
)

func TestStableFingerprintIgnoresReplacementPodIdentity(t *testing.T) {
	first := StableFingerprint(event.Event{
		Resource: "pod", Namespace: "prod", PodName: "api-abc", OwnerKind: "Deployment",
		Reason: "CrashLoopBackOff", ContainerName: "api",
	}, "api", nil)
	replacement := StableFingerprint(event.Event{
		Resource: "pod", Namespace: "prod", PodName: "api-def", OwnerKind: "Deployment",
		Reason: "CrashLoopBackOff", ContainerName: "api",
	}, "api", nil)
	if first == "" || first != replacement {
		t.Fatalf("replacement Pods must share fingerprint: %q != %q", first, replacement)
	}
	other := StableFingerprint(event.Event{
		Resource: "pod", Namespace: "prod", PodName: "api-ghi", OwnerKind: "Deployment",
		Reason: "ImagePullBackOff", ContainerName: "api",
	}, "api", nil)
	if first == other {
		t.Fatal("different failure reasons must not share a fingerprint")
	}
}
