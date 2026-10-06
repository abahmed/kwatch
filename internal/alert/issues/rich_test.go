package issues

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestBodyIsCommonMark(t *testing.T) {
	got := Body(providertest.Rich())
	for _, want := range []string{"🔴 **payments** in **shop**",
		"`boom *x* @​channel`",
		"```\nkubectl logs payments -n shop\n```"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRichBodyJiraWiki(t *testing.T) {
	got := RichBody(providertest.Rich(), notification.JiraDialect(),
		"{noformat}")
	for _, want := range []string{"🔴 *payments* in *shop*",
		"{{boom *x* @​channel}}",
		"{noformat}\nkubectl logs payments -n shop\n{noformat}"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
