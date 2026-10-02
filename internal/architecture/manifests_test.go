package architecture

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	var deployment, claim *unstructured.Unstructured
	var hasLeaseRole bool
	for i := range documents {
		document := &documents[i]
		switch document.GetKind() {
		case "Deployment":
			deployment = document
		case "PersistentVolumeClaim":
			claim = document
		case "PodDisruptionBudget":
			t.Fatal("a single-writer deployment must not have a PDB")
		case "Role":
			if document.GetName() == "kwatch-leader-election" {
				hasLeaseRole = true
			}
			if document.GetName() == "kwatch-configmap-manager" {
				t.Fatal("state lives on disk; no ConfigMap write access")
			}
		}
	}
	if deployment == nil || claim == nil {
		t.Fatal("raw deployment must include Deployment and state PVC")
	}
	replicas, _, _ := unstructured.NestedFieldNoCopy(
		deployment.Object, "spec", "replicas",
	)
	strategy, _, _ := unstructured.NestedString(
		deployment.Object, "spec", "strategy", "type",
	)
	if fmt.Sprint(replicas) != "1" || strategy != "Recreate" {
		t.Fatalf("deployment = %v replicas, %q; want 1, Recreate",
			replicas, strategy)
	}
	assertField(t, deployment, "spec", "template", "spec",
		"terminationGracePeriodSeconds")
	assertField(t, deployment, "spec", "template", "spec", "securityContext")
	assertField(t, deployment, "spec", "template", "spec", "tolerations")
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
	assertProbePath(t, container, "livenessProbe", "/healthz")
	assertProbePath(t, container, "readinessProbe", "/availabilityz")
	if !hasLeaseRole {
		t.Fatal("raw deployment is missing the Lease lock RBAC")
	}
}

func TestHelmDefaultMatchesRawDeploymentSafety(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is unavailable")
	}
	root := repositoryRoot(t)
	raw := readManifestFile(t, filepath.Join(root, "deploy", "deploy.yaml"))
	rendered := renderHelm(t, filepath.Join(root, "deploy", "chart"))
	rawDeployment := findManifestKind(t, raw, "Deployment")
	chartDeployment := findManifestKind(t, rendered, "Deployment")

	compareManifestField(t, rawDeployment, chartDeployment,
		"spec", "replicas")
	compareManifestField(t, rawDeployment, chartDeployment,
		"spec", "strategy", "type")
	compareManifestField(t, rawDeployment, chartDeployment,
		"spec", "template", "spec", "terminationGracePeriodSeconds")
	compareManifestField(t, rawDeployment, chartDeployment,
		"spec", "template", "spec", "securityContext")
	compareManifestField(t, rawDeployment, chartDeployment,
		"spec", "template", "spec", "tolerations")
	compareContainerProbe(t, rawDeployment, chartDeployment, "livenessProbe")
	compareContainerProbe(t, rawDeployment, chartDeployment, "readinessProbe")
}

func TestHelmDefaultIsSingleWriterWithState(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is unavailable")
	}
	root := repositoryRoot(t)
	rendered := renderHelm(t, filepath.Join(root, "deploy", "chart"))
	deployment := findManifestKind(t, rendered, "Deployment")
	strategy, _, err := unstructured.NestedString(
		deployment.Object, "spec", "strategy", "type",
	)
	if err != nil || strategy != "Recreate" {
		t.Fatalf("strategy = %q, want Recreate", strategy)
	}
	findManifestKind(t, rendered, "PersistentVolumeClaim")
	for _, document := range rendered {
		if document.GetKind() == "PodDisruptionBudget" {
			t.Fatal("a single-writer deployment must not have a PDB")
		}
	}
}

func readManifestFile(t *testing.T, path string) []unstructured.Unstructured {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	return decodeManifestDocuments(t, file)
}

func renderHelm(
	t *testing.T,
	chart string,
	args ...string,
) []unstructured.Unstructured {
	t.Helper()
	commandArgs := []string{"template", "architecture", chart}
	commandArgs = append(commandArgs, args...)
	output, err := exec.Command("helm", commandArgs...).CombinedOutput()
	if err != nil {
		t.Fatalf("helm template failed: %v\n%s", err, output)
	}
	return decodeManifestDocuments(t, bytes.NewReader(output))
}

func findManifestKind(
	t *testing.T,
	documents []unstructured.Unstructured,
	kind string,
) *unstructured.Unstructured {
	t.Helper()
	for index := range documents {
		if documents[index].GetKind() == kind {
			return &documents[index]
		}
	}
	t.Fatalf("manifest is missing %s", kind)
	return nil
}

func compareManifestField(
	t *testing.T,
	left *unstructured.Unstructured,
	right *unstructured.Unstructured,
	fields ...string,
) {
	t.Helper()
	leftValue, leftFound, leftErr := unstructured.NestedFieldNoCopy(
		left.Object, fields...,
	)
	rightValue, rightFound, rightErr := unstructured.NestedFieldNoCopy(
		right.Object, fields...,
	)
	if leftErr != nil || rightErr != nil || !leftFound || !rightFound {
		t.Fatalf("manifest field %v is missing", fields)
	}
	if fmt.Sprint(leftValue) != fmt.Sprint(rightValue) {
		t.Fatalf("manifest field %v differs: %v != %v", fields,
			leftValue, rightValue)
	}
}

func compareContainerProbe(
	t *testing.T,
	left *unstructured.Unstructured,
	right *unstructured.Unstructured,
	probe string,
) {
	t.Helper()
	leftContainers, _, _ := unstructured.NestedSlice(
		left.Object, "spec", "template", "spec", "containers",
	)
	rightContainers, _, _ := unstructured.NestedSlice(
		right.Object, "spec", "template", "spec", "containers",
	)
	leftContainer := leftContainers[0].(map[string]interface{})
	rightContainer := rightContainers[0].(map[string]interface{})
	leftPath, _, _ := unstructured.NestedString(
		leftContainer, probe, "httpGet", "path",
	)
	rightPath, _, _ := unstructured.NestedString(
		rightContainer, probe, "httpGet", "path",
	)
	if leftPath != rightPath {
		t.Fatalf("%s path differs: %q != %q", probe, leftPath, rightPath)
	}
}

func assertProbePath(
	t *testing.T,
	container map[string]interface{},
	probe, want string,
) {
	t.Helper()
	path, found, err := unstructured.NestedString(
		container, probe, "httpGet", "path",
	)
	if err != nil || !found || path != want {
		t.Fatalf("%s path = %q, want %q", probe, path, want)
	}
}

func decodeManifestDocuments(
	t *testing.T,
	reader io.Reader,
) []unstructured.Unstructured {
	t.Helper()
	decoder := utilyaml.NewYAMLOrJSONDecoder(reader, 4096)
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
