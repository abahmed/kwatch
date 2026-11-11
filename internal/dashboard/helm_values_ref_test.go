package dashboard

import (
	"io/fs"
	"os"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

// helmValueRef is a chart value the page names, such as rbac.mode or
// watch.secrets: a top-level chart key and one key under it.
var helmValueRef = regexp.MustCompile(
	`\b(rbac|watch|crd|persistence|networkPolicy|dashboard)\.([a-zA-Z]+)\b`)

// Advice on the page that names a chart value names one the chart has,
// so nobody is told to set a value that does not exist.
func TestPageNamesOnlyRealHelmValues(t *testing.T) {
	raw, err := os.ReadFile("../../deploy/chart/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]any
	if err := yaml.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	files, err := fs.Glob(web, "web/js/*.js")
	if err != nil || len(files) == 0 {
		t.Fatalf("no scripts: %v", err)
	}
	for _, name := range files {
		b, err := fs.ReadFile(web, name)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range helmValueRef.FindAllStringSubmatch(
			string(b), -1) {
			top, _ := values[m[1]].(map[string]any)
			if _, ok := top[m[2]]; !ok {
				t.Errorf("%s names %s, which values.yaml lacks",
					name, m[0])
			}
		}
	}
}

// The chart widens the ClusterRole for rbac.mode=full only while
// watch.secrets is true, so advice to set rbac.mode: full must name
// watch.secrets too: alone it changes nothing by default.
func TestFullRBACAdviceNamesWatchSecrets(t *testing.T) {
	chart, err := os.ReadFile("../../deploy/chart/templates/rbac.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`if and \(eq [^\n]*"full"\) \$secrets`).
		Match(chart) {
		t.Fatal("rbac.yaml no longer ties full to watch.secrets: " +
			"review the advice in web/js/evidence.js and this test")
	}
	b, err := fs.ReadFile(web, "web/js/evidence.js")
	if err != nil {
		t.Fatal(err)
	}
	advice := regexp.MustCompile(
		`(?s)var DENIED_OTHER = (.*?);\n`).FindSubmatch(b)
	if advice == nil {
		t.Fatal("evidence.js has no DENIED_OTHER")
	}
	full := regexp.MustCompile(`rbac\.mode: full`).Match(advice[1])
	secrets := regexp.MustCompile(`watch\.secrets: true`).Match(advice[1])
	if full && !secrets {
		t.Error("the advice says rbac.mode: full without " +
			"watch.secrets: true")
	}
}
