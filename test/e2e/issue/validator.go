package issue

import (
	"fmt"
	"strings"
)

var rejectedFragments = []string{
	"kubectl ", "docker ", "curl ", "wget ", "bash ", "sh -c",
	"$(", "authorization:", "password:", "passwd:", "token:",
	"clientsecret", "private_key", "-----begin ", "https://", "http://",
	"secret:", "secretname:", "secretref:",
}

// ValidateInput rejects data that could turn issue text into an execution or
// credential boundary. Marked blocks are data only; callers must still review
// the resulting sanitized manifest before committing a permanent scenario.
func ValidateInput(value string) error {
	lower := strings.ToLower(value)
	for _, fragment := range rejectedFragments {
		if strings.Contains(lower, fragment) {
			return fmt.Errorf("unsupported or sensitive content %q", fragment)
		}
	}
	if strings.Contains(value, "`") {
		return fmt.Errorf("shell-style backticks are not allowed")
	}
	return nil
}

// ValidateImage accepts only an image reference that the caller already built
// and loaded locally. The issue parser never pulls or resolves an image.
func ValidateImage(image string) error {
	if strings.TrimSpace(image) == "" {
		return fmt.Errorf("local workload image is required")
	}
	if err := ValidateInput(image); err != nil {
		return fmt.Errorf("image: %w", err)
	}
	if strings.Contains(image, "/") || strings.Contains(image, "@") {
		return fmt.Errorf("only a local image name is allowed")
	}
	return nil
}
