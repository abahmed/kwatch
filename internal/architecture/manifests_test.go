package architecture

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

func TestRawManifestsAreValidYAML(t *testing.T) {
	root := repositoryRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "deploy", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no raw deployment manifests found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			contents, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer contents.Close()
			documents := decodeManifestDocuments(t, contents)
			if len(documents) == 0 {
				t.Fatal("manifest contains no Kubernetes documents")
			}
		})
	}
}

func TestRawDeploymentHasProductionShape(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "deploy", "deploy.yaml")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	documents := decodeManifestDocuments(t, file)
	var deployment, pdb *unstructured.Unstructured
	var hasLeaseRole, hasConfigMapRole bool
	for i := range documents {
		document := &documents[i]
		switch document.GetKind() {
		case "Deployment":
			deployment = document
		case "PodDisruptionBudget":
			pdb = document
		case "Role":
			if document.GetName() == "kwatch-leader-election" {
				hasLeaseRole = true
			}
			if document.GetName() == "kwatch-configmap-manager" {
				hasConfigMapRole = true
			}
		}
	}
	if deployment == nil || pdb == nil {
		t.Fatal("raw deployment must include Deployment and PDB")
	}
	assertField(t, deployment, "spec", "strategy", "type")
	strategy, _, _ := unstructured.NestedString(
		deployment.Object, "spec", "strategy", "type",
	)
	if strategy != "RollingUpdate" {
		t.Fatalf("deployment strategy = %q, want RollingUpdate", strategy)
	}
	assertField(t, deployment, "spec", "template", "spec",
		"terminationGracePeriodSeconds")
	assertField(t, deployment, "spec", "template", "spec", "securityContext")
	assertField(t, deployment, "spec", "template", "spec",
		"topologySpreadConstraints")
	assertField(t, deployment, "spec", "template", "spec", "containers")
	containers, _, _ := unstructured.NestedSlice(
		deployment.Object, "spec", "template", "spec", "containers",
	)
	if len(containers) == 0 {
		t.Fatal("deployment has no containers")
	}
	container, ok := containers[0].(map[string]interface{})
	if !ok {
		t.Fatal("deployment container has unexpected shape")
	}
	for _, probe := range []string{"livenessProbe", "readinessProbe"} {
		if _, ok := container[probe]; !ok {
			t.Fatalf("container is missing %s", probe)
		}
	}
	if !hasLeaseRole || !hasConfigMapRole {
		t.Fatal("raw deployment is missing required persistence/election RBAC")
	}
}

func decodeManifestDocuments(
	t *testing.T,
	file *os.File,
) []unstructured.Unstructured {
	t.Helper()
	decoder := utilyaml.NewYAMLOrJSONDecoder(file, 4096)
	var documents []unstructured.Unstructured
	for {
		var document map[string]interface{}
		if err := decoder.Decode(&document); err != nil {
			if err == io.EOF {
				break
			}
			t.Fatalf("invalid YAML: %v", err)
		}
		if len(document) > 0 {
			documents = append(documents,
				unstructured.Unstructured{Object: document})
		}
	}
	return documents
}

func assertField(
	t *testing.T,
	document *unstructured.Unstructured,
	fields ...string,
) {
	t.Helper()
	value, found, err := unstructured.NestedFieldNoCopy(
		document.Object, fields...,
	)
	if err != nil || !found || value == nil {
		t.Fatalf("manifest is missing %s: %v", fmt.Sprint(fields), err)
	}
}
