package explain

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// exitedWith records the last exit code of the API's containers and,
// when line is set, the error line their previous log showed.
func exitedWith(f *fixture, api []inventory.EntityID, code float64,
	line string,
) {
	for _, pod := range api {
		attrs := map[string]inventory.Value{
			kube.AttrLastExitCode: inventory.Number(code)}
		if line != "" {
			attrs[kube.AttrLastErrorLine] = inventory.Text(line)
		}
		f.apply(inventory.Observation{Kind: inventory.Observed,
			Entity: containerOf(pod), Attributes: attrs})
	}
}

// A crash whose log is not read yet will say why it crashed: a name its
// configuration holds is not blamed before that.
func TestMissingServiceFromConfigWaitsForTheCrashLog(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	exitedWith(f, api, 1, "")
	failAPI(f, api, "back-off restarting failed container")

	c, ok := f.explain().CauseOf(containerOf(api[0]))
	assert.False(t, ok && c.Root == missing,
		"a configured name is no cause while the crash log is unread")
}

// The log names another failure: the configured name does not outrank
// it.
func TestMissingServiceFromConfigYieldsToTheLoggedError(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	exitedWith(f, api, 1,
		"panic: dial tcp: lookup cache.example.com: no such host")
	failAPI(f, api, "back-off restarting failed container")

	c, ok := f.explain().CauseOf(containerOf(api[0]))
	assert.False(t, ok && c.Root == missing)
}

func TestMissingServiceFromConfigExplainsALogThatNamesIt(t *testing.T) {
	f := newFixture(t)
	api, missing := missingFixture(f, "shop/paymnts:8080")
	exitedWith(f, api, 1, "fatal: cannot reach paymnts:8080")
	failAPI(f, api, "back-off restarting failed container")

	requireRoot(t, f, containerOf(api[0]), missing.String())
}
