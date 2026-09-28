package persistence

import (
	"context"
	"regexp"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestClusterIDFromUIDIsDeterministic(t *testing.T) {
	uid := "550e8400-e29b-41d4-a716-446655440000"
	id1 := ClusterIDFromUID(uid)
	id2 := ClusterIDFromUID(uid)
	if id1 != id2 {
		t.Fatalf("ClusterIDFromUID not deterministic: %q != %q", id1, id2)
	}
}

func TestClusterIDFromUIDDifferForDifferentUIDs(t *testing.T) {
	id1 := ClusterIDFromUID("550e8400-e29b-41d4-a716-446655440000")
	id2 := ClusterIDFromUID("550e8400-e29b-41d4-a716-446655440001")
	if id1 == id2 {
		t.Fatalf("different UIDs should produce different IDs: %q == %q",
			id1, id2)
	}
}

func TestClusterIDFromUIDMatchesUUIDv4Format(t *testing.T) {
	uuidv4Regex := regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-` +
			`[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	)
	uid := "550e8400-e29b-41d4-a716-446655440000"
	id := ClusterIDFromUID(uid)
	if !uuidv4Regex.MatchString(id) {
		t.Fatalf("ClusterIDFromUID %q is not valid UUIDv4", id)
	}
}

func TestEnsureClusterIDWithKubeSystemNamespace(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: "kube-system",
				UID:  "550e8400-e29b-41d4-a716-446655440000",
			},
		},
	)
	sm := newTestManager(client, "kwatch")

	clusterID, err := sm.EnsureClusterID(context.Background())
	if err != nil {
		t.Fatalf("EnsureClusterID: %v", err)
	}

	expected := ClusterIDFromUID("550e8400-e29b-41d4-a716-446655440000")
	if clusterID != expected {
		t.Fatalf("cluster ID = %q, want %q", clusterID, expected)
	}
}

func TestEnsureClusterIDWithoutKubeSystemNamespace(t *testing.T) {
	client := fake.NewSimpleClientset()
	sm := newTestManager(client, "kwatch")

	clusterID, err := sm.EnsureClusterID(context.Background())
	if err != nil {
		t.Fatalf("EnsureClusterID: %v", err)
	}

	uuidv4Regex := regexp.MustCompile(
		`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-` +
			`[89ab][0-9a-f]{3}-[0-9a-f]{12}$`,
	)
	if !uuidv4Regex.MatchString(clusterID) {
		t.Fatalf("cluster ID %q is not valid UUIDv4", clusterID)
	}
}
