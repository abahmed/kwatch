package main

import (
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/abahmed/kwatch/internal/app"
	"github.com/abahmed/kwatch/internal/flight"
)

// replayExportUsage is shown for a wrong command line.
const replayExportUsage = "replay-export --since 2h [--until 30m] " +
	"[--state path] --out file.jsonl"

// runReplayExport writes a window of the flight recording as a log the
// scenario library loads. It reads a copy of the state file.
func runReplayExport(
	args []string, out, errOut io.Writer, now func() time.Time,
) int {
	fs := flag.NewFlagSet("replay-export", flag.ContinueOnError)
	fs.SetOutput(errOut)
	since := fs.String("since", "", "start: a duration back (2h) or RFC 3339")
	until := fs.String("until", "", "end: a duration back or RFC 3339")
	state := fs.String("state", app.StatePath(), "state file to read")
	dest := fs.String("out", "", "file to write")
	if err := fs.Parse(args); err != nil || *dest == "" || fs.NArg() > 0 {
		return usageError(errOut, "replay-export", replayExportUsage,
			args)
	}
	from, to, err := flight.ParseWindow(*since, *until, now())
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "replay-export: %v\n", err)
		return 2
	}
	sum, err := flight.Export(flight.ExportOptions{
		StatePath: *state, Since: from, Until: to, Out: *dest,
	})
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "replay-export: %v\n", err)
		return 1
	}
	_, _ = fmt.Fprintf(out, "wrote %d entries (%d baseline) from %s to %s "+
		"to %s\n", sum.Entries, sum.Baseline,
		sum.Start.Format(time.RFC3339), sum.End.Format(time.RFC3339), *dest)
	return 0
}
