// Command e2eissue converts a sanitized GitHub issue reproduction into E2E
// fixture files. Issue content is data only; this command never executes it.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/go-github/v55/github"

	"github.com/abahmed/kwatch/test/e2e/issue"
)

type options struct {
	issueURL  string
	bodyFile  string
	output    string
	namespace string
	image     string
	sourceRef string
}

type metadata struct {
	IssueURL  string `json:"issueURL,omitempty"`
	SourceRef string `json:"sourceRef,omitempty"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
}

func main() {
	options := parseOptions()
	body, err := readBody(options)
	if err != nil {
		fatal(err)
	}
	blocks, err := issue.Parse(body)
	if err != nil {
		fatal(err)
	}
	if err := issue.Validate(blocks); err != nil {
		fatal(err)
	}
	resources, err := issue.SanitizeResources(
		blocks.Resources, options.namespace, options.image,
	)
	if err != nil {
		fatal(err)
	}
	if err := writeBundle(options, blocks, resources); err != nil {
		fatal(err)
	}
}

func parseOptions() options {
	var result options
	flag.StringVar(&result.issueURL, "issue", "", "GitHub issue URL")
	flag.StringVar(&result.bodyFile, "body-file", "", "local issue body file")
	flag.StringVar(&result.output, "output", "", "fixture output directory")
	flag.StringVar(
		&result.namespace, "namespace", "kwatch-issue", "sanitized namespace",
	)
	flag.StringVar(
		&result.image, "image", "kwatch-e2e-workload:test",
		"approved local workload image",
	)
	flag.StringVar(&result.sourceRef, "source-ref", "main", "source ref to record")
	flag.Parse()
	if result.output == "" || (result.issueURL == "" && result.bodyFile == "") ||
		(result.issueURL != "" && result.bodyFile != "") {
		flag.Usage()
		fatal(fmt.Errorf("provide exactly one of -issue or -body-file and -output"))
	}
	return result
}

func readBody(options options) (string, error) {
	if options.bodyFile != "" {
		body, err := os.ReadFile(filepath.Clean(options.bodyFile))
		return string(body), err
	}
	owner, repository, number, err := parseIssueURL(options.issueURL)
	if err != nil {
		return "", err
	}
	client := github.NewClient(nil)
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		client = client.WithAuthToken(token)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	issueItem, _, err := client.Issues.Get(ctx, owner, repository, number)
	if err != nil {
		return "", fmt.Errorf("fetch GitHub issue: %w", err)
	}
	return issueItem.GetBody(), nil
}

func parseIssueURL(value string) (string, string, int, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host != "github.com" {
		return "", "", 0, fmt.Errorf("issue must be a github.com URL")
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 4 || parts[2] != "issues" {
		return "", "", 0, fmt.Errorf(
			"issue URL must look like /owner/repo/issues/123",
		)
	}
	number, err := strconv.Atoi(parts[3])
	if err != nil || number <= 0 {
		return "", "", 0, fmt.Errorf("issue number is invalid")
	}
	return parts[0], parts[1], number, nil
}

func writeBundle(options options, blocks issue.Blocks, resources string) error {
	if err := os.MkdirAll(options.output, 0o755); err != nil {
		return fmt.Errorf("create output: %w", err)
	}
	files := map[string][]byte{
		"resources.yaml": []byte(resources),
		"config.yaml":    []byte(blocks.Config),
		"expectation.md": []byte(blocks.Expectation),
		"metadata.json": mustJSON(metadata{
			IssueURL: options.issueURL, SourceRef: options.sourceRef,
			Namespace: options.namespace, Image: options.image,
		}),
	}
	for name, payload := range files {
		if err := os.WriteFile(
			filepath.Join(options.output, name), payload, 0o600,
		); err != nil {
			return fmt.Errorf("write %s: %w", name, err)
		}
	}
	return nil
}

func mustJSON(value any) []byte {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(err)
	}
	return append(payload, '\n')
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(2)
}
