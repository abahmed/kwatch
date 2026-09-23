package main

import (
	"fmt"
	"io"
	"os"

	"github.com/abahmed/kwatch/internal/message"
)

const maxArtifactInputBytes = 32 << 20

func main() {
	data, err := io.ReadAll(io.LimitReader(
		os.Stdin, maxArtifactInputBytes+1,
	))
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	if len(data) > maxArtifactInputBytes {
		_, _ = os.Stderr.WriteString(fmt.Sprintf(
			"artifact exceeds %d-byte limit\n", maxArtifactInputBytes,
		))
		os.Exit(1)
	}
	_, err = os.Stdout.WriteString(message.RedactEvidence(string(data)))
	if err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
