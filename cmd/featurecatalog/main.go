package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/abahmed/kwatch/internal/feature"
)

const catalogHeader = "# kwatch feature catalog v2"

func main() {
	output := flag.String("output", "", "path to the feature catalog file to write")
	flag.Parse()
	if *output == "" {
		fmt.Fprintln(os.Stderr, "-output is required")
		os.Exit(2)
	}
	if err := feature.ValidateCatalog(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	content := render()
	if err := os.WriteFile(*output, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func render() string {
	lines := []string{catalogHeader}
	for _, definition := range feature.Catalog() {
		dependencies := make([]string, 0, len(definition.Dependencies))
		for _, dependency := range definition.Dependencies {
			dependencies = append(dependencies, string(dependency))
		}
		lines = append(lines, strings.Join([]string{
			string(definition.ID),
			string(definition.Lifecycle),
			definition.Description,
			strings.Join(dependencies, ","),
			string(feature.TierOf(definition.ID)),
		}, "|"))
	}
	return strings.Join(lines, "\n") + "\n"
}
