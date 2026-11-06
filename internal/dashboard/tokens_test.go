package dashboard

import (
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// brandTokens is where the brand's own tokens.css is in a checkout of
// the workspace; KWATCH_BRAND_TOKENS names another place.
const brandTokens = "../../../kwatch.dev/packages/brand/tokens.css"

var (
	declaredProp = regexp.MustCompile(`(--[a-z0-9-]+)\s*:`)
	usedProp     = regexp.MustCompile(`var\(\s*(--[a-z0-9-]+)`)
	colorLiteral = regexp.MustCompile(`rgba?\(|hsla?\(`)
)

func cssFiles(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := fs.WalkDir(web, "web", func(p string, d fs.DirEntry,
		err error) error {
		if err == nil && !d.IsDir() && path.Ext(p) == ".css" {
			b, rerr := fs.ReadFile(web, p)
			out[path.Base(p)] = string(b)
			return rerr
		}
		return err
	})
	require.NoError(t, err)
	return out
}

// The dashboard's tokens.css is the brand's file. When the brand
// package is next to this repository (the workspace), the two must be
// identical; apart from that, no other stylesheet (but look.css, the
// high-contrast overrides) may declare a brand token, so the dashboard cannot drift from it.
func TestTokensAreTheBrandTokens(t *testing.T) {
	files := cssFiles(t)
	tokens, ok := files["tokens.css"]
	require.True(t, ok, "web/tokens.css is missing")
	src := os.Getenv("KWATCH_BRAND_TOKENS")
	if src == "" {
		src = brandTokens
	}
	if brand, err := os.ReadFile(src); err == nil {
		assert.Equal(t, string(brand), tokens,
			"web/tokens.css differs from %s: copy the brand file", src)
	} else {
		t.Logf("brand tokens not found at %s: compared by name only", src)
	}
	brandNames := map[string]bool{}
	for _, m := range declaredProp.FindAllStringSubmatch(tokens, -1) {
		brandNames[m[1]] = true
	}
	require.Contains(t, brandNames, "--control-border")
	for name, css := range files {
		// look.css overrides tokens on purpose: that is what high
		// contrast is.
		if name == "tokens.css" || name == "look.css" {
			continue
		}
		for prop := range brandNames {
			assert.False(t, declares(css, prop),
				"%s redeclares the brand token %s", name, prop)
		}
	}
}

// declares reports a custom property declaration (not a var() use or
// a property name inside another declaration).
func declares(css, prop string) bool {
	re := regexp.MustCompile(`(^|[{;\s])` + regexp.QuoteMeta(prop) +
		`\s*:`)
	return re.MatchString(css)
}

// Every var(--x) names a property that something declares, and no
// stylesheet but tokens.css and the high-contrast overrides holds a
// color literal.
func TestStylesheetsUseOnlyDeclaredTokens(t *testing.T) {
	files := cssFiles(t)
	declared := map[string]bool{}
	for _, css := range files {
		for _, m := range declaredProp.FindAllStringSubmatch(css, -1) {
			declared[m[1]] = true
		}
	}
	for name, css := range files {
		for _, m := range usedProp.FindAllStringSubmatch(css, -1) {
			assert.True(t, declared[m[1]], "%s uses undeclared %s",
				name, m[1])
		}
		if name != "tokens.css" {
			assert.False(t, colorLiteral.MatchString(css),
				"%s has an rgb()/hsl() color: use a token", name)
		}
		assert.False(t, strings.Contains(css, "--ink-border"), name)
	}
}

// Shadows belong to the hero alert card of the marketing site only: a
// dashboard surface has a 1px --line border instead.
func TestStylesheetsHaveNoDropShadows(t *testing.T) {
	blur := regexp.MustCompile(`box-shadow:\s*(inset\s+)?-?\d+px\s+` +
		`-?\d+px\s+[1-9]\d*px`)
	for name, css := range cssFiles(t) {
		if name == "tokens.css" {
			continue
		}
		assert.False(t, blur.MatchString(css),
			"%s has a blurred drop shadow", name)
	}
}
