package dashboard

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
)

// formField is an <input>, <select> or <textarea> start tag;
// fieldKey is an id or name attribute on it.
var (
	formField = regexp.MustCompile(`<(?:input|select|textarea)\b[^>]*>`)
	fieldKey  = regexp.MustCompile(`\s(?:id|name)="[^"]+"`)
)

// Browsers flag a form field with neither an id nor a name (it can't
// be autofilled), so every field in the shell carries one.
func TestStaticFormFieldsHaveAnIDOrName(t *testing.T) {
	b, err := fs.ReadFile(web, "web/index.html")
	if err != nil {
		t.Fatal(err)
	}
	fields := formField.FindAllString(string(b), -1)
	if len(fields) == 0 {
		t.Fatal("no form fields found in the shell")
	}
	for _, f := range fields {
		if !fieldKey.MatchString(f) {
			t.Errorf("form field without id or name: %s",
				strings.TrimSpace(f))
		}
	}
}

// A release that changes only the offline page changes the service
// worker too, so browsers install it and copy the new page.
func TestWorkerChangesWithTheOfflinePage(t *testing.T) {
	build := func(page string) string {
		t.Helper()
		root := fstest.MapFS{
			"web/index.html":           {Data: []byte("<p>")},
			"web/app.css":              {Data: []byte("body{}")},
			"web/favicon.svg":          {Data: []byte("<svg/>")},
			"web/manifest.webmanifest": {Data: []byte("{}")},
			"web/sw.js": {Data: []byte(
				`var CACHE = "c-{{version:offline.html}}";`)},
			"web/offline.html": {Data: []byte(page)},
		}
		a, err := buildAssets(root)
		if err != nil {
			t.Fatal(err)
		}
		return string(a.root[workerName])
	}
	one, two := build("<p>one"), build("<p>two")
	if one == two || strings.Contains(one, "{{version:") {
		t.Fatalf("worker did not follow the page: %q, %q", one, two)
	}
}
