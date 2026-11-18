package inventory

import (
	"math/rand"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// maxAttrEntryBytes is the size of one stored attribute. A large cluster
// keeps millions of them, so a field that grows it fails here first.
const maxAttrEntryBytes = 96

func TestAttrEntryStaysCompact(t *testing.T) {
	assert.LessOrEqual(t, unsafe.Sizeof(attrEntry{}),
		uintptr(maxAttrEntryBytes))
}

// refAttr is the straightforward store the compact one must match: the
// public Attribute kept whole, with the source that last set it.
type refAttr struct {
	source string
	attr   Attribute
}

// refObserve applies one Observed observation the way the attribute
// store has always behaved.
func refObserve(ref map[string]refAttr, o Observation) {
	for name, entry := range ref {
		if _, still := o.Attributes[name]; entry.source == o.Source &&
			!still {
			delete(ref, name)
		}
	}
	for name, value := range o.Attributes {
		entry, found := ref[name]
		entry.source = o.Source
		switch {
		case found && entry.attr.Value.Equal(value):
			entry.attr.Updated = o.At
		case found:
			next := Attribute{Value: value, Since: o.At, Updated: o.At}
			if _, isBool := value.AsBool(); isBool {
				next.Flips = withFlip(entry.attr.Flips, o.At)
			}
			entry.attr = next
		default:
			entry.attr = Attribute{Value: value, Since: o.At, Updated: o.At}
		}
		ref[name] = entry
	}
}

func randomValue(rng *rand.Rand, base time.Time) Value {
	switch rng.Intn(5) {
	case 0:
		return Bool(rng.Intn(2) == 0)
	case 1:
		return Text([]string{"Running", "Pending", ""}[rng.Intn(3)])
	case 2:
		return Number(float64(rng.Intn(3)))
	case 3:
		return Time(base.Add(time.Duration(rng.Intn(2)) * time.Hour))
	default:
		return Value{}
	}
}

func randomObservation(
	rng *rand.Rand, id EntityID, base time.Time, step int,
) Observation {
	zones := []*time.Location{time.UTC, time.FixedZone("plus3", 3*3600)}
	at := base.Add(time.Duration(step*7+rng.Intn(5)) * time.Second).
		In(zones[rng.Intn(len(zones))])
	attrs := map[string]Value{}
	for _, name := range []string{"ready", "phase", "restarts", "node"} {
		if rng.Intn(3) > 0 {
			attrs[name] = randomValue(rng, base)
		}
	}
	return Observation{Kind: Observed,
		Source: []string{"k8s", "stats", "probe"}[rng.Intn(3)],
		At:     at, Entity: id, Attributes: attrs}
}

func TestAttributeStoreMatchesReferenceBehaviour(t *testing.T) {
	base := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	for seed := int64(1); seed <= 40; seed++ {
		rng := rand.New(rand.NewSource(seed))
		m := NewModel(Options{})
		id := CoreID("pod", "shop", "api-0")
		ref := map[string]refAttr{}
		for step := 0; step < 120; step++ {
			o := randomObservation(rng, id, base, step)
			_, err := m.Apply(o)
			require.NoError(t, err)
			refObserve(ref, o)
			assertMatchesReference(t, m, id, ref)
		}
	}
}

func assertMatchesReference(
	t *testing.T, m *Model, id EntityID, ref map[string]refAttr,
) {
	t.Helper()
	want := make(map[string]Attribute, len(ref))
	for name, entry := range ref {
		want[name] = entry.attr
	}
	entity, ok := m.Entity(id)
	require.True(t, ok)
	require.Equal(t, want, entity.Attributes)
	for name, attr := range want {
		got, found := m.Attribute(id, name)
		require.True(t, found)
		require.Equal(t, attr, got)
		require.True(t, got.Updated.Equal(attr.Updated))
		require.Equal(t, attr.Updated.Location(), got.Updated.Location())
	}
}

func TestAttributeNamesAreInterned(t *testing.T) {
	m := NewModel(Options{})
	at := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	ids := []EntityID{CoreID("pod", "shop", "a"), CoreID("pod", "shop", "b")}
	for _, id := range ids {
		// A fresh string each time, as decoding an object yields.
		name := string([]byte("phase"))
		_, err := m.Apply(Observation{Kind: Observed, Source: "k8s",
			At: at, Entity: id,
			Attributes: map[string]Value{name: Text("Running")}})
		require.NoError(t, err)
	}
	first := m.records[ids[0]].attrs[0].name.Value()
	second := m.records[ids[1]].attrs[0].name.Value()
	assert.Equal(t, unsafe.StringData(first), unsafe.StringData(second))
}

func TestFlipsStayUnallocatedUntilAFlip(t *testing.T) {
	m := NewModel(Options{})
	id := CoreID("pod", "shop", "api-0")
	start := time.Date(2026, 10, 7, 10, 0, 0, 0, time.UTC)
	observeBool(m, id, start, true)
	observeBool(m, id, start.Add(time.Minute), true)
	assert.Nil(t, m.records[id].attrs[0].flips)
	observeBool(m, id, start.Add(2*time.Minute), false)
	require.NotNil(t, m.records[id].attrs[0].flips)
	attribute, _ := m.Attribute(id, "ready")
	assert.Equal(t, []time.Time{start.Add(2 * time.Minute)}, attribute.Flips)
}
