package inventory

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func roundTrip(t *testing.T, in Observation) Observation {
	t.Helper()
	data, err := json.Marshal(in)
	require.NoError(t, err)
	var out Observation
	require.NoError(t, json.Unmarshal(data, &out), string(data))
	return out
}

func TestObservationJSONRoundTripsEveryKind(t *testing.T) {
	at := time.Date(2026, 9, 29, 14, 0, 0, 123456789, time.UTC)
	pod := CoreID("pod", "shop", "payments-1")
	node := CoreID("node", "", "n1")
	cases := map[string]Observation{
		"observed": {
			Kind: Observed, Source: "k8s-informer", At: at, Entity: pod,
			UID: "uid-1", Attributes: map[string]Value{
				"phase":    Text("Running"),
				"restarts": Number(4.5),
				"ready":    Bool(false),
				"healthy":  Bool(true),
				"since":    Time(at),
			},
		},
		"related": {
			Kind: Related, Source: "k8s-informer", At: at, Entity: pod,
			Relation: RunsOn, Targets: []EntityID{node},
		},
		"changed": {
			Kind: Changed, Source: "k8s-informer", At: at, Entity: pod,
			Change: Change{
				Entity: pod, At: at, Actor: "alice", Revision: "3",
				Created: true, Deleted: true, Fields: []FieldChange{{
					Path: "image", Before: "app:1", After: "app:2",
				}},
			},
		},
		"gone": {Kind: Gone, Source: "k8s-informer", At: at, Entity: node},
		"noted": {
			Kind: Noted, Source: "k8s-events", At: at, Entity: pod,
			Note: Note{
				At: at, Source: "kubelet", Reason: "BackOff",
				Message: "Back-off restarting", Count: 3, Warning: true,
			},
		},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, in, roundTrip(t, in))
		})
	}
}

func TestObservationJSONUsesStableNames(t *testing.T) {
	data, err := json.Marshal(Observation{
		Kind: Gone, Entity: CoreID("node", "", "n1"),
	})
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"kind":"gone","entity":{"kind":"node","name":"n1"}}`,
		string(data))
}

func TestObservationJSONNormalisesTimesToUTC(t *testing.T) {
	zone := time.FixedZone("plus2", 2*3600)
	at := time.Date(2026, 9, 29, 16, 0, 0, 0, zone)
	out := roundTrip(t, Observation{
		Kind: Gone, At: at, Entity: CoreID("node", "", "n1"),
	})
	assert.True(t, out.At.Equal(at))
	assert.Equal(t, time.UTC, out.At.Location())
}

func TestObservationJSONRejectsInvalidInput(t *testing.T) {
	for name, input := range map[string]string{
		"missing kind": `{"entity":{"kind":"pod","name":"a"}}`,
		"unknown kind": `{"kind":"exploded","entity":{"name":"a"}}`,
		"bad value": `{"kind":"observed","entity":{"name":"a"},` +
			`"attributes":{"x":{"text":"a","number":1}}}`,
		"empty value": `{"kind":"observed","entity":{"name":"a"},` +
			`"attributes":{"x":{}}}`,
		"bad time": `{"kind":"observed","entity":{"name":"a"},` +
			`"attributes":{"x":{"time":"yesterday"}}}`,
		"not an object": `[1]`,
	} {
		t.Run(name, func(t *testing.T) {
			var out Observation
			assert.Error(t, json.Unmarshal([]byte(input), &out))
		})
	}
}

func TestObservationKindMarshalRejectsUnknown(t *testing.T) {
	_, err := ObservationKind(99).MarshalText()
	assert.Error(t, err)
	assert.Equal(t, "unknown", ObservationKind(0).String())
	assert.Equal(t, "noted", Noted.String())
}

func TestValueJSONRoundTrip(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	for name, in := range map[string]Value{
		"zero":       {},
		"text":       Text("hello"),
		"empty text": Text(""),
		"number":     Number(-1.25),
		"zero num":   Number(0),
		"true":       Bool(true),
		"false":      Bool(false),
		"time":       Time(at),
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(in)
			require.NoError(t, err)
			var out Value
			require.NoError(t, json.Unmarshal(data, &out))
			assert.True(t, in.Equal(out), "%s -> %s", in.AsText(), data)
		})
	}
}

func TestValueJSONRejectsNonFiniteNumbers(t *testing.T) {
	_, err := json.Marshal(Number(math.NaN()))
	assert.Error(t, err)
	_, err = json.Marshal(Number(math.Inf(1)))
	assert.Error(t, err)
	_, err = json.Marshal(Value{kind: 42})
	assert.Error(t, err)
}
