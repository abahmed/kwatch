package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/abahmed/kwatch/test/e2e/issue"
)

type outputMetadata struct {
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Source    string `json:"source"`
}

func main() {
	issueFile := flag.String("issue-file", "", "path to a GitHub issue body")
	outputDir := flag.String(
		"output-dir", "", "directory for reviewed reproduction data",
	)
	namespace := flag.String("namespace", "kwatch-issue", "scenario namespace")
	image := flag.String("image", "kwatch-e2e-workload:test",
		"already-loaded local workload image")
	flag.Parse()
	if err := run(*issueFile, *outputDir, *namespace, *image); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(issueFile, outputDir, namespace, image string) error {
	if strings.TrimSpace(issueFile) == "" || strings.TrimSpace(outputDir) == "" {
		return fmt.Errorf("--issue-file and --output-dir are required")
	}
	if err := issue.ValidateImage(image); err != nil {
		return err
	}
	body, err := os.ReadFile(issueFile)
	if err != nil {
		return fmt.Errorf("read issue body: %w", err)
	}
	reproduction, err := issue.Parse(string(body))
	if err != nil {
		return fmt.Errorf("parse issue reproduction: %w", err)
	}
	resources, err := issue.SanitizeResources(
		reproduction.ResourcesYAML, namespace, image,
	)
	if err != nil {
		return fmt.Errorf("sanitize resources: %w", err)
	}
	if err := os.MkdirAll(outputDir, 0o750); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	files := map[string][]byte{
		"config.yaml":     reproduction.ConfigYAML,
		"resources.yaml":  resources,
		"expectation.txt": []byte(reproduction.Expectation + "\n"),
	}
	for name, content := range files {
		path := filepath.Join(outputDir, name)
		if err := os.WriteFile(path, content, 0o640); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	metadata, err := json.MarshalIndent(outputMetadata{
		Namespace: namespace,
		Image:     image,
		Source:    "sanitized issue blocks; not executed",
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode metadata: %w", err)
	}
	metadata = append(metadata, '\n')
	if err := os.WriteFile(
		filepath.Join(outputDir, "metadata.json"), metadata, 0o640,
	); err != nil {
		return fmt.Errorf("write metadata: %w", err)
	}
	return nil
}
