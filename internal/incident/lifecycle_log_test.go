package incident

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"
)

// A resolve is logged at the default level with its root, reason and
// how long the incident lasted, so kwatch's decisions can be audited
// from its logs, not only its opens.
func TestIncidentResolveIsLogged(t *testing.T) {
	out := captureKlog(t)
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	failOnce(r, web, 0, 4*time.Minute)
	klog.Flush()

	var line string
	for _, l := range strings.Split(out.String(), "\n") {
		if strings.Contains(l, `"incident resolved"`) {
			line = l
		}
	}
	assert.NotEmpty(t, line, "no resolve line")
	assert.Contains(t, line, `operation="resolve"`)
	assert.Contains(t, line, `incident="`+r.idOf(web.Entity)+`"`)
	assert.Contains(t, line, `name="web"`)
	assert.Contains(t, line, `reason=`)
	assert.Contains(t, line, `lasted=`)
	assert.Contains(t, line, `quiet=false`)
}
