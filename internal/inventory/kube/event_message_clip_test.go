package kube_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const loadBalancerWarning = "Failed deploy model due to operation error " +
	"Elastic Load Balancing v2: DeleteLoadBalancer, https response " +
	"error StatusCode: 400, RequestID: " +
	"1725b5e1-de3d-4e8b-8b52-4a8a9a21a0aa, ResourceInUse: Load " +
	"balancer 'arn:aws:elasticloadbalancing:eu-west-1:" +
	"653511765142:loadbalancer/net/k8s-kubeaddo-internal-d2ea1523e3/" +
	"863dfe7121eb7b98' cannot be deleted because it is currently " +
	"associated with another service"

// An error ends with its reason ("... cannot be deleted because it is
// currently associated with another service"), so a long event message
// loses its middle and keeps its beginning and its end.
func TestEventNoteKeepsTheEndOfALongMessage(t *testing.T) {
	ev := warningEvent("Service", "")
	ev.Message = loadBalancerWarning
	is := assert.New(t)
	is.Greater(len(ev.Message), 330)

	observation, ok := kube.EventNote(ev, fixedTime())

	is.True(ok)
	got := observation.Note.Message
	is.LessOrEqual(len(got), 256+len("…"))
	is.True(strings.HasPrefix(got, "Failed deploy model"), got)
	is.True(strings.HasSuffix(got, "cannot be deleted because it is "+
		"currently associated with another service"), got)
	is.Contains(got, "…")
}

// A message that fits is kept whole, and a cut never splits a rune.
func TestEventNoteClipKeepsShortMessagesAndRunes(t *testing.T) {
	ev := warningEvent("Pod", "")
	ev.Message = "restarting"
	short, _ := kube.EventNote(ev, fixedTime())
	assert.Equal(t, "restarting", short.Note.Message)

	ev.Message = strings.Repeat("é", 300) + " the end"
	long, _ := kube.EventNote(ev, fixedTime())
	assert.True(t, strings.HasSuffix(long.Note.Message, " the end"))
	assert.True(t, strings.ToValidUTF8(long.Note.Message, "?") ==
		long.Note.Message)
}
