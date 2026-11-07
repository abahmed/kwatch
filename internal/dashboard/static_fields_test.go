package dashboard

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"
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
