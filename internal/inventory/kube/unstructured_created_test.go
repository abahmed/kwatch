package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A CertificateSigningRequest records the creation time the cluster
// gives it, so a wait for its approval counts from then.
func TestUnstructuredSchemaRecordsCreationOfCertificateRequests(t *testing.T) {
	created := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "certificates.k8s.io/v1",
		"kind":       "CertificateSigningRequest",
		"spec":       map[string]any{"signerName": "example.com/signer"},
	}}
	u.SetName("csr-1")
	u.SetCreationTimestamp(metav1.NewTime(created))

	desc, ok := kube.NewUnstructuredSchema("certificates.k8s.io",
		"CertificateSigningRequest").Describe(u)

	assert.True(t, ok)
	assert.True(t, created.Equal(desc.Attributes[kube.AttrCreated].AsTime()))
}

// A ResourceClaim records its creation time too, so a pending pod's wait
// for a device counts from when the claim was made.
func TestUnstructuredSchemaRecordsCreationOfResourceClaims(t *testing.T) {
	created := time.Date(2026, 3, 1, 9, 0, 0, 0, time.UTC)
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "resource.k8s.io/v1",
		"kind":       "ResourceClaim",
		"spec":       map[string]any{},
	}}
	u.SetName("train-0-gpu")
	u.SetNamespace("ml")
	u.SetCreationTimestamp(metav1.NewTime(created))

	desc, ok := kube.NewUnstructuredSchema("resource.k8s.io",
		"ResourceClaim").Describe(u)

	assert.True(t, ok)
	assert.True(t, created.Equal(desc.Attributes[kube.AttrCreated].AsTime()))
}
