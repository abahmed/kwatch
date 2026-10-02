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
	registerLimits(flags, &limits)
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

// registerLimits binds one flag per threshold. Limits with a production
// goal default to it; a negative value disables a check.
func registerLimits(flags *flag.FlagSet, limits *scorecard.Thresholds) {
	flags.Float64Var(&limits.MaxPerHour, "max-per-hour", -1,
		"fail above this mean number of notifications per hour")
	flags.Float64Var(&limits.MaxPerIncident, "max-per-incident", -1,
		"fail above this mean number of messages per incident")
	flags.Float64Var(&limits.MaxUnchangedPercent, "max-unchanged-pct",
		scorecard.GoalUnchangedPercent,
		"fail above this percent of updates without a visible change")
	flags.IntVar(&limits.MaxRecreated, "max-recreated", -1,
		"fail above this many re-opened (flapping) incidents")
	flags.IntVar(&limits.MaxRepeatedResolves, "max-repeated-resolves",
		scorecard.GoalRepeatedResolves,
		"fail above this many repeated recovery messages")
	flags.Float64Var(&limits.MaxUnknownCausePct, "max-unknown-cause-pct", -1,
		"fail above this percent of notifications without a cause")
	flags.IntVar(&limits.MaxCircularCause, "max-circular-cause", -1,
		"fail above this many causes blaming the failing object itself")
	flags.Float64Var(&limits.MaxRecreatedPercent, "max-recreated-pct",
		scorecard.GoalRecreatedPercent,
		"fail above this percent of incidents re-created after resolving")
	flags.IntVar(&limits.MaxPeakPerHour, "max-peak-per-hour",
		scorecard.GoalPeakPerHour,
		"fail above this many notifications inside any one hour")
	flags.IntVar(&limits.MaxMessagesPerIncident,
		"max-messages-per-incident", scorecard.GoalMessagesPerIncident,
		"fail above this many messages for any one incident")
	flags.IntVar(&limits.MaxMessagesPerIncidentP95,
		"max-messages-per-incident-p95",
		scorecard.GoalMessagesPerIncidentP95,
		"fail above this 95th percentile of messages per incident")
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
