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
	setMemoryLimitFromEnv()
	return runWithFlags()
}

// runWithFlags parses CLI flags, dispatches to subcommands (version, lint)
// and otherwise starts the server.
func runWithFlags() int {
	klog.InitFlags(nil)
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.Short())
		return 0
	}

	args := flag.Args()
	return runCommand(args, os.Stdout, os.Stderr, app.Run)
}

// runCommand dispatches an already-parsed command. Keeping process I/O and
// the server entrypoint behind parameters makes command behavior testable;
// main remains the only place that terminates the process.
func runCommand(
	args []string,
	out, errOut io.Writer,
	runApp func() int,
) int {
	if len(args) > 0 {
		switch args[0] {
		case "version":
			return runVersion(args[1:], out, errOut)
		case "lint":
			strict, check, ok := parseLintArgs(args[1:])
			if !ok {
				return usageError(errOut, "lint",
					"[--strict] [--check]", args[1:])
			}
			return runLint(strict, check, out, errOut)
		default:
			// A typo must not silently start a full monitoring instance.
			if _, err := fmt.Fprintf(
				errOut,
				"unknown command %q (commands: version, lint)\n",
				args[0],
			); err != nil {
				return 2
			}
			return 2
		}
	}

	return runApp()
}

// parseLintArgs accepts --strict/strict and --check/check; any other
// argument is rejected so a typo cannot pass as a clean lint.
func parseLintArgs(args []string) (strict, check, ok bool) {
	for _, a := range args {
		switch a {
		case "--strict", "strict":
			strict = true
		case "--check", "check":
			check = true
		default:
			return false, false, false
		}
	}
	return strict, check, true
}

func usageError(errOut io.Writer, command, usage string, args []string) int {
	if _, err := fmt.Fprintf(
		errOut, "%s: unexpected arguments %q (usage: %s %s)\n",
		command, args, command, usage,
	); err != nil {
		return 2
	}
	return 2
}

func runVersion(args []string, out, errOut io.Writer) int {
	asJSON := false
	switch {
	case len(args) == 0:
	case len(args) == 1 && args[0] == "--json":
		asJSON = true
	default:
		return usageError(errOut, "version", "[--json]", args)
	}
	if !asJSON {
		info := version.Current()
		if _, err := fmt.Fprintf(
			out, "version %s (commit %s, built %s)\n",
			info.Version, info.Commit, info.BuildDate,
		); err != nil {
			return 1
		}
		return 0
	}
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
	return 0
}
