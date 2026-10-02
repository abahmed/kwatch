package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func typed(apiVersion, kind string, spec, status map[string]any,
) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion, "kind": kind,
		"metadata": map[string]any{"name": "x", "namespace": "ns"},
		"spec":     spec, "status": status,
	}}
}

func cond(kind, status, reason string) map[string]any {
	return map[string]any{"type": kind, "status": status, "reason": reason}
}

func TestUnstructuredSchemaListsListenerProblems(t *testing.T) {
	gw := typed("gateway.networking.k8s.io/v1", "Gateway", nil,
		map[string]any{"listeners": []any{
			map[string]any{"name": "https", "conditions": []any{
				cond("Programmed", "False", "InvalidCertificateRef"),
				cond("Accepted", "True", "Accepted"),
			}},
			map[string]any{"name": "http", "conditions": []any{
				cond("OverlappingTLSConfig", "True", "OverlappingHostnames"),
				cond("Conflicted", "False", "NoConflicts"),
			}},
			map[string]any{"name": "ok", "conditions": []any{
				cond("Programmed", "True", "Programmed"),
			}},
		}})
	s := kube.NewUnstructuredSchema("gateway.networking.k8s.io", "Gateway")

	d, ok := s.Describe(gw)

	assert.True(t, ok)
	assert.Equal(t,
		"listener http: OverlappingTLSConfig=True (OverlappingHostnames); "+
			"listener https: Programmed=False (InvalidCertificateRef)",
		text(d, kube.AttrListenerProblems))
}

func TestUnstructuredSchemaMergesPolicyAncestors(t *testing.T) {
	policy := typed("gateway.networking.k8s.io/v1", "BackendTLSPolicy", nil,
		map[string]any{"ancestors": []any{
			map[string]any{
				"ancestorRef": map[string]any{"name": "gw"},
				"conditions": []any{map[string]any{
					"type": "ResolvedRefs", "status": "False",
					"reason": "InvalidCACertificateRef", "message": "no ca",
				}},
			},
		}})
	s := kube.NewUnstructuredSchema("gateway.networking.k8s.io",
		"BackendTLSPolicy")

	d, _ := s.Describe(policy)

	key := kube.ConditionKey("ResolvedRefs")
	assert.Equal(t, "False", text(d, key))
	assert.Equal(t, "ancestor gw: no ca", text(d, key+kube.AttrConditionMessage))
}

func TestUnstructuredSchemaRecordsCertificateIssue(t *testing.T) {
	s := kube.NewUnstructuredSchema("certificates.k8s.io",
		"CertificateSigningRequest")
	spec := map[string]any{"signerName": "kubernetes.io/kubelet-serving"}
	pending, _ := s.Describe(typed("certificates.k8s.io/v1",
		"CertificateSigningRequest", spec, map[string]any{}))
	issued, _ := s.Describe(typed("certificates.k8s.io/v1",
		"CertificateSigningRequest", spec,
		map[string]any{"certificate": "LS0t"}))

	got, _ := pending.Attributes[kube.AttrCertificateIssued].AsBool()
	assert.False(t, got)
	got, _ = issued.Attributes[kube.AttrCertificateIssued].AsBool()
	assert.True(t, got)
	assert.Equal(t, "kubernetes.io/kubelet-serving",
		text(issued, kube.AttrSignerName))
}

func TestUnstructuredSchemaRecordsClaimAllocationAndDevices(t *testing.T) {
	s := kube.NewUnstructuredSchema("resource.k8s.io", "ResourceClaim")
	claim := typed("resource.k8s.io/v1", "ResourceClaim", nil,
		map[string]any{
			"allocation": map[string]any{"devices": map[string]any{}},
			"devices": []any{map[string]any{
				"driver": "gpu.example.com", "pool": "node-1",
				"device": "gpu-0", "conditions": []any{map[string]any{
					"type": "Ready", "status": "False", "message": "ecc",
				}},
			}},
		})

	d, _ := s.Describe(claim)

	allocated, _ := d.Attributes[kube.AttrAllocated].AsBool()
	assert.True(t, allocated)
	assert.Equal(t, "device node-1/gpu-0: ecc",
		text(d, kube.ConditionKey("Ready")+kube.AttrConditionMessage))

	pending, _ := s.Describe(typed("resource.k8s.io/v1", "ResourceClaim",
		nil, map[string]any{}))
	allocated, _ = pending.Attributes[kube.AttrAllocated].AsBool()
	assert.False(t, allocated)
}
