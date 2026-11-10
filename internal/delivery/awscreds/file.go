package awscreds

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// maxTokenFileBytes caps a token file read. Projected service account
// tokens and Pod Identity tokens are a few kilobytes.
const maxTokenFileBytes = 64 << 10

// readToken reads a token file afresh: kubelet rotates these files, so
// the content is never cached. The error names the file, never its
// content.
func readToken(path string) (string, error) {
	f, err := os.Open(path) // #nosec G304 -- path is the operator's env
	if err != nil {
		return "", fmt.Errorf("read token file: %w", err)
	}
	defer func() { _ = f.Close() }()
	data, err := io.ReadAll(io.LimitReader(f, maxTokenFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	if len(data) > maxTokenFileBytes {
		return "", fmt.Errorf("token file %s is larger than %d bytes",
			path, maxTokenFileBytes)
	}
	token := strings.TrimSpace(string(data))
	if token == "" {
		return "", fmt.Errorf("token file %s is empty", path)
	}
	return token, nil
}
