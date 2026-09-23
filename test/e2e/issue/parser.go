package issue

import (
	"fmt"
	"strings"
)

// Reproduction contains only declarative, marked issue input. It never
// contains executable commands or credentials from the issue body.
type Reproduction struct {
	ConfigYAML      []byte
	ResourcesYAML   []byte
	Expectation     string
	ReportedImage   string
	ReportedVersion string
	SourceSHA       string
}

// Parse extracts the three supported issue blocks. Everything outside these
// blocks is intentionally ignored, including comments and shell snippets.
func Parse(body string) (Reproduction, error) {
	config, err := extractBlock(body, "kwatch-config", true)
	if err != nil {
		return Reproduction{}, err
	}
	resources, err := extractBlock(body, "kwatch-resources", true)
	if err != nil {
		return Reproduction{}, err
	}
	expectation, err := extractBlock(body, "kwatch-expectation", false)
	if err != nil {
		return Reproduction{}, err
	}
	if err := ValidateInput(config); err != nil {
		return Reproduction{}, fmt.Errorf("config block: %w", err)
	}
	if err := ValidateInput(resources); err != nil {
		return Reproduction{}, fmt.Errorf("resources block: %w", err)
	}
	if err := ValidateInput(expectation); err != nil {
		return Reproduction{}, fmt.Errorf("expectation block: %w", err)
	}
	return Reproduction{
		ConfigYAML:    []byte(config),
		ResourcesYAML: []byte(resources),
		Expectation:   strings.TrimSpace(expectation),
	}, nil
}

func extractBlock(body, name string, requireFence bool) (string, error) {
	lines := strings.Split(body, "\n")
	marker := "" + name
	for index, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if !strings.Contains(lower, marker) ||
			strings.Contains(lower, "/"+marker) {
			continue
		}
		content := lines[index+1:]
		if requireFence {
			return fencedContent(content, name)
		}
		return plainContent(content, name), nil
	}
	return "", fmt.Errorf("missing marked block %q", name)
}

func fencedContent(lines []string, name string) (string, error) {
	start := -1
	for index, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			start = index + 1
			break
		}
	}
	if start < 0 {
		return "", fmt.Errorf("block %q must contain a fenced document", name)
	}
	for index := start; index < len(lines); index++ {
		if strings.HasPrefix(strings.TrimSpace(lines[index]), "```") {
			value := strings.TrimSpace(strings.Join(lines[start:index], "\n"))
			if value == "" {
				return "", fmt.Errorf("block %q is empty", name)
			}
			return value, nil
		}
	}
	return "", fmt.Errorf("block %q has no closing fence", name)
}

func plainContent(lines []string, name string) string {
	var selected []string
	for _, line := range lines {
		lower := strings.ToLower(strings.TrimSpace(line))
		if strings.Contains(lower, "kwatch-") &&
			!strings.Contains(lower, name) {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue
		}
		selected = append(selected, line)
	}
	return strings.TrimSpace(strings.Join(selected, "\n"))
}
