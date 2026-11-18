package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// servedAPI puts deployment api with two pods behind Service api, with
// ready of three endpoints, and returns the failing pods.
func servedAPI(s *shapeModel, ready int) []inventory.EntityID {
	n1 := s.node("n1", "amd64", "m5")
	rs := s.replicaSet("api-1", entity(kube.KindDeployment, "api"))
	pods := []inventory.EntityID{s.pod("api-a", rs, n1, false),
		s.pod("api-b", rs, n1, false)}
	svc := entity(kube.KindService, "api")
	s.set(svc, map[string]inventory.Value{})
	s.relate(svc, inventory.Selects, pods...)
	s.slice(svc, ready, 3)
	return pods
}

func TestUsersAffectedWhenTheRouteHasNoReadyEndpoint(t *testing.T) {
	s := newShapeModel(t)
	pods := servedAPI(s, 0)
	s.ingress("web", "shop.example.com", entity(kube.KindService, "api"))

	u := UsersOf(s.m, crashIncident(pods[0], pods...))

	require.NotNil(t, u)
	assert.Equal(t, UsersAffected, u.State)
	assert.Equal(t, "Users are affected: route shop.example.com → api "+
		"has 0 of 3 ready endpoints.", u.Text)
}

func TestUsersStillServedWhileEndpointsAreLeft(t *testing.T) {
	s := newShapeModel(t)
	pods := servedAPI(s, 2)
	s.ingress("web", "shop.example.com", entity(kube.KindService, "api"))

	u := UsersOf(s.m, crashIncident(pods[0], pods...))

	require.NotNil(t, u)
	assert.Equal(t, UsersServed, u.State)
	assert.Equal(t, "Users are still served: route shop.example.com → "+
		"api has 2 of 3 ready endpoints.", u.Text)
}

func TestUsersWithoutARouteNameTheCallers(t *testing.T) {
	s := newShapeModel(t)
	pods := servedAPI(s, 1)
	n1 := inventory.CoreID(kube.KindNode, "", "n1")
	for _, name := range []string{"orders", "billing"} {
		rs := s.replicaSet(name+"-1", entity(kube.KindDeployment, name))
		caller := s.pod(name+"-a", rs, n1, true)
		s.relate(caller, inventory.Calls, entity(kube.KindService, "api"))
	}

	u := UsersOf(s.m, crashIncident(pods[0], pods...))

	require.NotNil(t, u)
	assert.Equal(t, UsersNoRoute, u.State)
	assert.Equal(t, "Service api has 1 of 3 ready endpoints; kwatch "+
		"does not see a route to it from outside the cluster; other "+
		"workloads that call it: billing, orders.", u.Text)
}

func TestUsersSayWhatKwatchDoesNotSee(t *testing.T) {
	s := newShapeModel(t)
	pods := servedAPI(s, 1)
	u := UsersOf(s.m, crashIncident(pods[0], pods...))
	require.NotNil(t, u)
	assert.Equal(t, "Service api has 1 of 3 ready endpoints; kwatch "+
		"does not see a route to it from outside the cluster or "+
		"another workload that calls it.", u.Text)

	s2 := newShapeModel(t)
	n1 := s2.node("n1", "amd64", "m5")
	rs := s2.replicaSet("worker-1", entity(kube.KindDeployment, "worker"))
	pod := s2.pod("worker-a", rs, n1, false)
	u = UsersOf(s2.m, crashIncident(pod, pod))
	require.NotNil(t, u)
	assert.Equal(t, UsersNoService, u.State)
	assert.Equal(t, "kwatch does not see a Service or a route in "+
		"front of it.", u.Text)

	assert.Nil(t, UsersOf(s2.m, crashIncident(n1)), "nothing that serves")
	assert.Nil(t, UsersOf(nil, crashIncident(pod, pod)))
}

// A route to a Service kwatch does not see, or whose endpoints it does
// not see, is not called affected or served.
func TestUsersDoNotGuessWhatKwatchCannotCount(t *testing.T) {
	s := newShapeModel(t)
	n1 := s.node("n1", "amd64", "m5")
	rs := s.replicaSet("api-1", entity(kube.KindDeployment, "api"))
	pod := s.pod("api-a", rs, n1, false)
	p := crashIncident(pod, pod)
	p.Impact = []inventory.EntityID{entity(kube.KindService, "gone")}
	s.ingress("web", "shop.example.com", entity(kube.KindService, "gone"))

	u := UsersOf(s.m, p)

	require.NotNil(t, u)
	assert.Equal(t, UsersUnknown, u.State)
	assert.Equal(t, "kwatch cannot tell whether users are affected: "+
		"route shop.example.com → gone is a Service kwatch does not "+
		"see.", u.Text)

	svc := entity(kube.KindService, "api")
	s.set(svc, map[string]inventory.Value{})
	s.relate(svc, inventory.Selects, pod)
	p.Impact = nil
	s.ingress("web", "shop.example.com", svc)
	u = UsersOf(s.m, p)
	require.NotNil(t, u)
	assert.Equal(t, UsersUnknown, u.State)
	assert.Equal(t, "kwatch cannot tell whether users are affected: "+
		"route shop.example.com → api has endpoints kwatch does not "+
		"see.", u.Text)
}
