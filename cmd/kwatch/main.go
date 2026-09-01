package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/app"
	"github.com/abahmed/kwatch/internal/version"
)

func main() {
	os.Exit(run())
}

func run() int {
	return runWithFlags()
}

// runWithFlags parses CLI flags, dispatches to subcommands (lint, replay), and
// otherwise starts the server. It is the main entry point after flag parsing.
func runWithFlags() int {
	klog.InitFlags(nil)
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Short())
		return 0
	}

	args := flag.Args()
	return runCommand(args, os.Stdin, os.Stdout, os.Stderr, app.Run)
}

// runCommand dispatches an already-parsed command. Keeping process I/O and
// the server entrypoint behind parameters makes command behavior testable;
// main remains the only place that terminates the process.
func runCommand(
	args []string,
	in io.Reader,
	out, errOut io.Writer,
	runApp func() int,
) int {
	if len(args) > 0 {
		switch args[0] {
		case "version":
			if len(args) > 1 && args[1] == "--json" {
				data, err := version.JSON()
				if err != nil {
					if _, writeErr := fmt.Fprintf(
						errOut, "could not encode version: %v\n", err,
					); writeErr != nil {
						return 1
					}
					return 1
				}
				if _, err := fmt.Fprintln(out, string(data)); err != nil {
					return 1
				}
			} else {
				info := version.Current()
				if _, err := fmt.Fprintf(
					out,
					"version %s (commit %s, built %s)\n",
					info.Version,
					info.Commit,
					info.BuildDate,
				); err != nil {
					return 1
				}
			}
			return 0
		case "lint":
			strict := false
			check := false
			for _, a := range args[1:] {
				if a == "--strict" || a == "strict" {
					strict = true
				}
				if a == "--check" || a == "check" {
					check = true
				}
			}
			return runLint(strict, check, out, errOut)
		case "replay":
			dryRun := false
			for _, a := range args[1:] {
				if a == "--dry-run" || a == "dry-run" {
					dryRun = true
				}
			}
			return runReplay(dryRun, in, out, errOut)
		}
	}

	return runApp()
}
