package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
)

func TestDeploymentSchemaDescribe(t *testing.T) {
	d := deployment("d1")
	schema := kube.DeploymentSchema()
	desc, ok := schema.Describe(d)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("deployment"), desc.ID.Kind)
	assert.Equal(t, "d1", desc.ID.Name)

	// Check generation attribute
	gen, ok := desc.Attributes["generation"]
	assert.True(t, ok)
	num, _ := gen.AsNumber()
	assert.Equal(t, 5.0, num)

	// Check template hash
	hash, ok := desc.Attributes["template.hash"]
	assert.True(t, ok)
	assert.NotEmpty(t, hash.AsText())
}

func TestStatefulSetSchemaDescribe(t *testing.T) {
	ss := statefulSet("ss1")
	schema := kube.StatefulSetSchema()
	desc, ok := schema.Describe(ss)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("statefulset"), desc.ID.Kind)
	assert.Equal(t, "ss1", desc.ID.Name)

	// Check replicas
	replicas, ok := desc.Attributes["replicas"]
	assert.True(t, ok)
	num, _ := replicas.AsNumber()
	assert.Equal(t, 3.0, num)
}

func TestDaemonSetSchemaDescribe(t *testing.T) {
	ds := daemonSet("ds1")
	schema := kube.DaemonSetSchema()
	desc, ok := schema.Describe(ds)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("daemonset"), desc.ID.Kind)

	// DaemonSet shouldn't have replica spec
	_, hasReplicas := desc.Attributes["replicas"]
	assert.False(t, hasReplicas)
}

func TestJobSchemaDescribe(t *testing.T) {
	j := job("j1")
	schema := kube.JobSchema()
	desc, ok := schema.Describe(j)

	assert.True(t, ok)
	assert.Equal(t, knowledge.Kind("job"), desc.ID.Kind)
	assert.Equal(t, "j1", desc.ID.Name)

	// Check template hash
	hash, ok := desc.Attributes["template.hash"]
	assert.True(t, ok)
	assert.NotEmpty(t, hash.AsText())
}

func TestJobSuspendDiff(t *testing.T) {
	old := job("j1")
	new := job("j1")
	suspend := true
	new.Spec.Suspend = &suspend

	schema := kube.JobSchema()
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "spec.suspend" {
			found = true
			assert.Equal(t, "false", c.Before)
			assert.Equal(t, "true", c.After)
		}
	}
	assert.True(t, found, "suspend change not found")
}

func TestWorkloadReferences(t *testing.T) {
	d := deployment("d1")
	d.Spec.Template.Spec.ServiceAccountName = "app-sa"

	schema := kube.DeploymentSchema()
	desc, ok := schema.Describe(d)
	assert.True(t, ok)

	refs := desc.Relations[knowledge.References]
	found := false
	for _, ref := range refs {
		if ref.Kind == knowledge.Kind("serviceaccount") &&
			ref.Name == "app-sa" {
			found = true
		}
	}
	assert.True(t, found,
		"service account reference not found")
}

func TestStatefulSetTemplateChange(t *testing.T) {
	old := statefulSet("ss1")
	new := statefulSet("ss1")
	new.Spec.Template.Spec.Containers[0].Image = "app:2.0"

	schema := kube.StatefulSetSchema()
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "containers[app].image" {
			found = true
			assert.Equal(t, "app:1.0", c.Before)
			assert.Equal(t, "app:2.0", c.After)
		}
	}
	assert.True(t, found, "image change not found")
}

func TestWorkloadGeneration(t *testing.T) {
	tt := []struct {
		name   string
		schema kube.Schema
		obj    any
	}{
		{
			name:   "deployment",
			schema: kube.DeploymentSchema(),
			obj:    deployment("d1"),
		},
		{
			name:   "replicaset",
			schema: kube.ReplicaSetSchema(),
			obj:    replicaSet("rs1"),
		},
		{
			name:   "statefulset",
			schema: kube.StatefulSetSchema(),
			obj:    statefulSet("ss1"),
		},
	}

	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			desc, ok := tc.schema.Describe(tc.obj)
			assert.True(t, ok)

			// Workloads should have generation
			gen, ok := desc.Attributes["generation"]
			assert.True(t, ok)
			assert.NotNil(t, gen)
		})
	}
}
