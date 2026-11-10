package scope

import (
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// clockedScope is a scope with a namespace selector and a clock the
// test moves.
func clockedScope(
	t *testing.T, selector string,
) (*Scope, *time.Time) {
	t.Helper()
	now := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	cfg := config.Config{NamespaceSelector: selector}
	s, err := New(config.RuntimeConfigFor(&cfg).Scope(), nil, false,
		func() time.Time { return now })
	require.NoError(t, err)
	return s, &now
}

func addNamespace(model *inventory.Model, name, labels string) {
	attrs := map[string]inventory.Value{}
	if labels != "" {
		attrs[kube.AttrLabels] = inventory.Text(labels)
	}
	_, _ = model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test",
		At:         time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC),
		Entity:     inventory.CoreID(kube.KindNamespace, "", name),
		Attributes: attrs,
	})
}

// "alerts!=off" must not let a namespace in just because its Namespace
// has not been read yet: it may well carry alerts=off. The decision is
// held, and taken from the labels once they are there.
func TestSelectorHoldsANamespaceItHasNotSeen(t *testing.T) {
	s, now := clockedScope(t, "alerts!=off")
	model := inventory.NewModel(inventory.Options{})
	sig := containerFinding("pay", "api-1", "app", "OOMKilled")

	assert.False(t, s.Allows(model, sig), "held, not decided on no labels")
	assert.True(t, s.Pending(model, sig))
	assert.Empty(t, s.UncheckedNamespaces())

	*now = now.Add(5 * time.Second)
	addNamespace(model, "pay", "alerts=off")
	assert.False(t, s.Allows(model, sig), "excluded by its labels")
	assert.False(t, s.Pending(model, sig), "a verdict now, not a hold")

	*now = now.Add(time.Hour)
	assert.False(t, s.Allows(model, sig))
	assert.Empty(t, s.UncheckedNamespaces())
}

// "alerts=on" must not go silent for good when a Namespace is late: the
// held finding is let through as soon as the labels match.
func TestSelectorDecidesOnceTheNamespaceIsSeen(t *testing.T) {
	s, _ := clockedScope(t, "alerts=on")
	model := inventory.NewModel(inventory.Options{})
	sig := containerFinding("shop", "web-1", "app", "OOMKilled")

	assert.False(t, s.Allows(model, sig))
	assert.True(t, s.Pending(model, sig))
	addNamespace(model, "shop", "alerts=on")
	assert.True(t, s.Allows(model, sig))
	assert.False(t, s.Pending(model, sig))
}

// A Namespace that cannot be read at all: after the hold its problems
// are reported, and the namespace is listed as not checked.
func TestSelectorLetsAnUnreadableNamespaceInAfterTheHold(t *testing.T) {
	s, now := clockedScope(t, "alerts=on")
	model := inventory.NewModel(inventory.Options{})
	sig := containerFinding("shop", "web-1", "app", "OOMKilled")

	assert.False(t, s.Allows(model, sig))
	*now = now.Add(NamespaceHold - time.Second)
	assert.False(t, s.Allows(model, sig))
	assert.True(t, s.Pending(model, sig))

	*now = now.Add(time.Second)
	assert.False(t, s.Pending(model, sig), "the hold is over")
	assert.True(t, s.Allows(model, sig))
	assert.True(t, s.NamespaceAllowed(model, "shop"))
	assert.Equal(t, []string{"shop"}, s.UncheckedNamespaces())

	// Readable after all: the labels decide again and the gap closes.
	addNamespace(model, "shop", "alerts=off")
	assert.False(t, s.Allows(model, sig))
	assert.Empty(t, s.UncheckedNamespaces())
}

// A Namespace without labels is known: the selector judges it at once.
func TestSelectorJudgesAKnownNamespaceWithoutLabels(t *testing.T) {
	s, _ := clockedScope(t, "alerts!=off")
	model := inventory.NewModel(inventory.Options{})
	addNamespace(model, "shop", "")
	sig := containerFinding("shop", "web-1", "app", "OOMKilled")

	assert.True(t, s.Allows(model, sig))
	assert.False(t, s.Pending(model, sig))

	strict, _ := clockedScope(t, "alerts=on")
	assert.False(t, strict.Allows(model, sig))
	assert.False(t, strict.Pending(model, sig))
}

// The namespace lists decide before the selector: a namespace they
// refuse is out, never pending; and no selector means nothing to hold.
func TestPendingOnlyForTheSelector(t *testing.T) {
	now := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	cfg := config.Config{NamespaceSelector: "alerts=on",
		ForbiddenNamespaces: []string{"dev"}}
	s, err := New(config.RuntimeConfigFor(&cfg).Scope(), nil, false,
		func() time.Time { return now })
	require.NoError(t, err)
	model := inventory.NewModel(inventory.Options{})

	assert.False(t, s.NamespacePending(model, "dev"))
	assert.False(t, s.NamespacePending(model, ""))
	assert.True(t, s.NamespaceAllowed(model, ""))

	plain := newScope(t, config.Config{})
	assert.False(t, plain.NamespacePending(model, "shop"))
	assert.Nil(t, plain.UncheckedNamespaces())
	var none *Scope
	assert.False(t, none.Pending(model,
		containerFinding("shop", "web-1", "app", "OOMKilled")))
}

// A held namespace's finding that another rule refuses (its reason) is
// refused for good: no hold makes it pending.
func TestPendingOnlyWhenTheHoldIsTheOnlyRefusal(t *testing.T) {
	now := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	cfg := config.Config{NamespaceSelector: "alerts=on",
		ForbiddenReasons: []string{"Evicted"}}
	s, err := New(config.RuntimeConfigFor(&cfg).Scope(), nil, false,
		func() time.Time { return now })
	require.NoError(t, err)
	model := inventory.NewModel(inventory.Options{})
	crash := containerFinding("shop", "web-1", "app", "OOMKilled")
	evicted := containerFinding("shop", "web-1", "app", "Evicted")

	assert.False(t, s.Allows(model, crash))
	assert.True(t, s.Pending(model, crash))
	assert.False(t, s.Pending(model, evicted), "refused by its reason")
}

// Past the namespaces it can time, the selector lets a new one in at
// once: its finding is not pending, as Allows does not hold it.
func TestPendingAgreesWithAllowsPastTheHoldCap(t *testing.T) {
	s, _ := clockedScope(t, "alerts=on")
	model := inventory.NewModel(inventory.Options{})
	for i := range maxUnknownNamespaces {
		s.Allows(model, containerFinding(
			"ns-"+strconv.Itoa(i), "web-1", "app", "OOMKilled"))
	}
	late := containerFinding("late", "web-1", "app", "OOMKilled")
	assert.False(t, s.Pending(model, late))
	assert.True(t, s.Allows(model, late))
}
