package main

import (
	"flag"
	"fmt"
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
	if len(args) > 0 {
		switch args[0] {
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
			runLint(strict, check)
			return 0
		case "replay":
			dryRun := false
			for _, a := range args[1:] {
				if a == "--dry-run" || a == "dry-run" {
					dryRun = true
				}
			}
			runReplay(dryRun)
			return 0
		}
	}

	return app.Run()
}
