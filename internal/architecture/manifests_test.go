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
	assertProbePath(t, container, "livenessProbe", "/healthz")
	assertProbePath(t, container, "readinessProbe", "/availabilityz")
	if !hasLeaseRole || !hasConfigMapRole {
		t.Fatal("raw deployment is missing required persistence/election RBAC")
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
	compareTopologySpread(t, rawDeployment, chartDeployment)
	compareContainerProbe(t, rawDeployment, chartDeployment, "livenessProbe")
	compareContainerProbe(t, rawDeployment, chartDeployment, "readinessProbe")
}

func TestHelmSingleReplicaOmitsDisruptionBudget(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is unavailable")
	}
	root := repositoryRoot(t)
	rendered := renderHelm(
		t, filepath.Join(root, "deploy", "chart"), "--set", "replicaCount=1",
	)
	deployment := findManifestKind(t, rendered, "Deployment")
	strategy, _, err := unstructured.NestedString(
		deployment.Object, "spec", "strategy", "type",
	)
	if err != nil || strategy != "Recreate" {
		t.Fatalf("single replica strategy = %q, want Recreate", strategy)
	}
	for _, document := range rendered {
		if document.GetKind() == "PodDisruptionBudget" {
			t.Fatal("single replica rendering must omit PodDisruptionBudget")
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

func compareTopologySpread(
	t *testing.T,
	left *unstructured.Unstructured,
	right *unstructured.Unstructured,
) {
	t.Helper()
	leftValues, _, _ := unstructured.NestedSlice(
		left.Object, "spec", "template", "spec", "topologySpreadConstraints",
	)
	rightValues, _, _ := unstructured.NestedSlice(
		right.Object, "spec", "template", "spec", "topologySpreadConstraints",
	)
	if len(leftValues) != len(rightValues) || len(leftValues) == 0 {
		t.Fatal("topology spread settings are not equivalent")
	}
	leftValue := leftValues[0].(map[string]interface{})
	rightValue := rightValues[0].(map[string]interface{})
	leftKey, _, _ := unstructured.NestedString(leftValue, "topologyKey")
	rightKey, _, _ := unstructured.NestedString(rightValue, "topologyKey")
	if leftKey != rightKey {
		t.Fatalf("topology key differs: %q != %q", leftKey, rightKey)
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
