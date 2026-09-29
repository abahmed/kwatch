// Command kwatch-scorecard replays a Kwatch audit log and reports
// notification quality KPIs, optionally failing when a threshold is
// exceeded.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/scorecard"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("kwatch-scorecard", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "-",
		"audit log: JSON lines or a log export CSV (- for stdin)")
	asJSON := flags.Bool("json", false, "print the report as JSON")
	limits := scorecard.NoThresholds()
	flags.Float64Var(&limits.MaxPerHour, "max-per-hour", -1,
		"fail above this many notifications per hour")
	flags.Float64Var(&limits.MaxPerProblem, "max-per-problem", -1,
		"fail above this many messages per problem")
	flags.Float64Var(&limits.MaxUnchangedPercent, "max-unchanged-pct", -1,
		"fail above this percent of updates without a visible change")
	flags.IntVar(&limits.MaxRecreated, "max-recreated", -1,
		"fail above this many re-opened (flapping) problems")
	flags.IntVar(&limits.MaxRepeatedResolves, "max-repeated-resolves", -1,
		"fail above this many repeated recovery messages")
	flags.Float64Var(&limits.MaxUnknownCausePct, "max-unknown-cause-pct", -1,
		"fail above this percent of notifications without a cause")
	flags.IntVar(&limits.MaxCircularCause, "max-circular-cause", -1,
		"fail above this many causes blaming the failing object itself")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	entries, err := readEntries(*input)
	if err != nil {
		_, _ = fmt.Fprintf(errOut, "read audit log: %v\n", err)
		return 2
	}
	report := scorecard.Score(entries)
	if err := write(out, report, *asJSON); err != nil {
		return 2
	}
	violations := limits.Violations(report)
	for _, violation := range violations {
		_, _ = fmt.Fprintf(errOut, "threshold exceeded: %s\n", violation)
	}
	if len(violations) > 0 {
		return 1
	}
	return 0
}

func readEntries(path string) ([]audit.Entry, error) {
	if path == "-" {
		return scorecard.Parse(os.Stdin)
	}
	// #nosec G304 -- operator-selected input file
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	return scorecard.Parse(file)
}

func write(out io.Writer, report scorecard.Report, asJSON bool) error {
	if asJSON {
		encoder := json.NewEncoder(out)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}
	_, err := io.WriteString(out, scorecard.Format(report))
	return err
}
