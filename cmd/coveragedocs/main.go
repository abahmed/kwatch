package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("coveragedocs", flag.ContinueOnError)
	output := flags.String("output", "docs/kubernetes-coverage.md",
		"path of the generated document")
	check := flags.Bool("check", false,
		"fail when the document is not up to date instead of writing it")
	root := flags.String("root", ".", "repository root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	doc, err := render(*root)
	if err != nil {
		return err
	}
	if !*check {
		return os.WriteFile(*output, doc, 0o644)
	}
	current, err := os.ReadFile(*output)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, doc) {
		return fmt.Errorf("%s is stale; run: go run ./cmd/coveragedocs",
			*output)
	}
	return nil
}
