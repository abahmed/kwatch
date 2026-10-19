package main

import (
	"bufio"
	"fmt"
	"io"

	"github.com/abahmed/kwatch/internal/redact"
)

// maxRedactLine bounds one input line. A longer line is an error, never
// passed through unredacted.
const maxRedactLine = 1 << 20

// runRedact copies in to out line by line through redact.Diagnostics. The
// installer pipes the diagnostics bundle through it, so the shell and Go
// share one redaction list.
func runRedact(
	args []string, in io.Reader, out, errOut io.Writer,
) int {
	if len(args) != 0 {
		return usageError(errOut, "redact", "< input", args)
	}
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), maxRedactLine)
	writer := bufio.NewWriter(out)
	for scanner.Scan() {
		if _, err := fmt.Fprintln(
			writer, redact.Diagnostics(scanner.Text()),
		); err != nil {
			return 1
		}
	}
	if err := scanner.Err(); err != nil {
		_ = writer.Flush()
		_, _ = fmt.Fprintf(errOut, "redact: %v\n", err)
		return 1
	}
	if err := writer.Flush(); err != nil {
		return 1
	}
	return 0
}
