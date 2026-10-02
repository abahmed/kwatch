package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestNodeSchemaDescribe(t *testing.T) {
	tt := []struct {
		name    string
		obj     any
		wantOK  bool
		checkFn func(*testing.T, kube.Description)
	}{
		{
			name:   "valid_node",
			obj:    node("node1"),
			wantOK: true,
			checkFn: func(t *testing.T, desc kube.Description) {
				assert.Equal(t, inventory.Kind("node"),
					desc.ID.Kind)
				assert.Equal(t, "node1", desc.ID.Name)
				// Cluster-scoped nodes have empty
				// namespace
				assert.Equal(t, "", desc.ID.Namespace)
				// Check ready attribute
				ready, ok := desc.Attributes["ready"]
				assert.True(t, ok)
				b, _ := ready.AsBool()
				assert.True(t, b)
			},
		},
		{
			name: "node_with_taints",
			obj: func() any {
				n := node("node1")
				n.Spec.Taints = []corev1.Taint{
					{
						Key:    "workload",
						Value:  "special",
						Effect: corev1.TaintEffectNoExecute,
					},
				}
				return n
			}(),
			wantOK: true,
			checkFn: func(t *testing.T, desc kube.Description) {
				taints, ok := desc.Attributes["taints"]
				assert.True(t, ok)
				text := taints.AsText()
				assert.Contains(t, text, "workload")
				assert.Contains(t, text, "special")
			},
		},
		{
			name: "node_with_labels",
			obj: func() any {
				n := node("node1")
				n.Labels = map[string]string{
					"kubernetes.io/hostname":           "node1",
					"node.kubernetes.io/instance-type": "t3.large",
					"topology.kubernetes.io/zone":      "us-west-2a",
				}
				return n
			}(),
			wantOK: true,
			checkFn: func(t *testing.T, desc kube.Description) {
				// Check instance type
				itype, ok := desc.Attributes["instance.type"]
				assert.True(t, ok)
				assert.Equal(t, "t3.large", itype.AsText())
				// Check zone relation
				rels := desc.Relations[inventory.PartOf]
				found := false
				for _, r := range rels {
					if r.Kind == inventory.Kind("zone") {
						found = true
					}
				}
				assert.True(t, found,
					"zone relation not found")
			},
		},
		{
			name:   "wrong_type",
			obj:    "not a node",
			wantOK: false,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			schema := kube.NodeSchema{}
			desc, ok := schema.Describe(tc.obj)
			assert.Equal(t, tc.wantOK, ok)
			if ok && tc.checkFn != nil {
				tc.checkFn(t, desc)
			}
		})
	}
}

func TestNodeSchemaDiff(t *testing.T) {
	tt := []struct {
		name    string
		old     any
		new     any
		wantLen int
		checkFn func(*testing.T, []inventory.FieldChange)
	}{
		{
			name:    "no_change",
			old:     node("node1"),
			new:     node("node1"),
			wantLen: 0,
		},
		{
			name: "unschedulable_changed",
			old:  node("node1"),
			new: func() any {
				n := node("node1")
				n.Spec.Unschedulable = true
				return n
			}(),
			wantLen: 1,
			checkFn: func(t *testing.T, changes []inventory.FieldChange) {
				assert.Equal(t,
					"spec.unschedulable",
					changes[0].Path)
				assert.Equal(t, "false",
					changes[0].Before)
				assert.Equal(t, "true",
					changes[0].After)
			},
		},
		{
			name: "taints_changed",
			old:  node("node1"),
			new: func() any {
				n := node("node1")
				n.Spec.Taints = []corev1.Taint{
					{
						Key:    "drain",
						Effect: corev1.TaintEffectNoExecute,
					},
				}
				return n
			}(),
			checkFn: func(t *testing.T, changes []inventory.FieldChange) {
				found := false
				for _, c := range changes {
					if c.Path == "spec.taints" {
						found = true
						assert.NotEqual(t,
							c.Before, c.After)
					}
				}
				assert.True(t, found,
					"taints change not found")
			},
		},
		{
			name: "kubelet_version_changed",
			old:  node("node1"),
			new: func() any {
				n := node("node1")
				n.Status.NodeInfo.KubeletVersion =
					"v1.26.0"
				return n
			}(),
			checkFn: func(t *testing.T, changes []inventory.FieldChange) {
				found := false
				for _, c := range changes {
					if c.Path ==
						"status.nodeInfo.kubeletVersion" {
						found = true
						assert.Equal(t, "v1.25.0",
							c.Before)
						assert.Equal(t, "v1.26.0",
							c.After)
					}
				}
				assert.True(t, found,
					"kubelet version change not found")
			},
		},
		{
			name: "heartbeat_status_only",
			old:  node("node1"),
			new: func() any {
				n := node("node1")
				// Update heartbeat
				n.Status.Conditions[0].
					LastTransitionTime = metav1.NewTime(
					fixedTime().Add(60))
				return n
			}(),
			wantLen: 0,
			checkFn: func(t *testing.T, changes []inventory.FieldChange) {
				// Status-only heartbeat is not a change
				assert.Len(t, changes, 0)
			},
		},
		{
			name:    "wrong_types",
			old:     "not a node",
			new:     "also not a node",
			wantLen: 0,
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			schema := kube.NodeSchema{}
			changes := schema.Diff(tc.old, tc.new)
			if tc.wantLen > 0 {
				assert.Len(t, changes, tc.wantLen)
			}
			if tc.checkFn != nil {
				tc.checkFn(t, changes)
			}
		})
	}
}

func TestNodeRelationTypes(t *testing.T) {
	schema := kube.NodeSchema{}
	types := schema.RelationTypes()
	assert.Contains(t, types, inventory.PartOf)
}

func TestNodeNodePool(t *testing.T) {
	tt := []struct {
		name     string
		labels   map[string]string
		wantPool string
	}{
		{
			name: "karpenter_nodepool",
			labels: map[string]string{
				"karpenter.sh/nodepool": "default",
			},
			wantPool: "default",
		},
		{
			name: "eks_nodegroup",
			labels: map[string]string{
				"eks.amazonaws.com/nodegroup": "ng1",
			},
			wantPool: "ng1",
		},
		{
			name: "gke_nodepool",
			labels: map[string]string{
				"cloud.google.com/gke-nodepool": "pool1",
			},
			wantPool: "pool1",
		},
		{
			name: "priority_karpenter_over_eks",
			labels: map[string]string{
				"karpenter.sh/nodepool":       "karp-pool",
				"eks.amazonaws.com/nodegroup": "eks-ng",
			},
			wantPool: "karp-pool",
		},
		{
			name:     "no_pool_label",
			labels:   map[string]string{},
			wantPool: "",
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			n := node("n1")
			n.Labels = tc.labels
			schema := kube.NodeSchema{}
			desc, _ := schema.Describe(n)
			rels := desc.Relations[inventory.PartOf]
			found := false
			for _, r := range rels {
				if r.Kind == inventory.Kind("nodepool") {
					found = true
					assert.Equal(t, tc.wantPool,
						r.Name)
				}
			}
			if tc.wantPool != "" {
				assert.True(t, found,
					"nodepool relation not found")
			}
		})
	}
}
