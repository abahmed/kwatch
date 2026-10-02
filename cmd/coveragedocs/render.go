package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const intro = `# Kubernetes coverage

This page is generated from code by ` + "`go run ./cmd/coveragedocs`" + `;
do not edit it by hand. It lists which Kubernetes resources kwatch watches
and how, and which failure modes the detectors can raise. For how findings
become incidents, see [architecture](./architecture.md).
`

func render(root string) ([]byte, error) {
	usage, err := scanDetectors(root)
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString(intro)
	b.WriteString("\n## Resources\n\n")
	b.WriteString("Watch modes: `full` keeps the whole object, `hashed` " +
		"keeps a content hash, `status` keeps status only, `metadata` " +
		"keeps metadata only, `excluded` is not watched.\n\n")
	b.WriteString("| Kind | Group | Watch mode |\n|:--|:--|:--|\n")
	for _, e := range sortedCatalog() {
		group := e.Group
		if group == "" {
			group = "core"
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", e.Kind, group, e.Mode)
	}
	b.WriteString("\n## Failure modes\n\n")
	b.WriteString("Health is taken from the severity each detector " +
		"assigns: warning is degraded, critical is failing; `varies` " +
		"means the severity depends on the observation.\n\n")
	b.WriteString("| Mode | Health | Reasons | Detectors |\n")
	b.WriteString("|:--|:--|:--|:--|\n")
	for _, m := range detection.Modes() {
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", m.Mode,
			usage.health(m.Reasons), codeList(m.Reasons),
			codeList(usage.detectors(m.Reasons)))
	}
	return []byte(b.String()), nil
}

func sortedCatalog() []kube.CoverageEntry {
	entries := kube.CoverageCatalog()
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Group < b.Group
	})
	return entries
}

func codeList(items []string) string {
	if len(items) == 0 {
		return "-"
	}
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = "`" + item + "`"
	}
	return strings.Join(quoted, ", ")
}
