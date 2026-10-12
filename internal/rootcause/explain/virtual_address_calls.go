package explain

import (
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A failed call that dials an address ("dial tcp 10.0.3.4:5432: i/o
// timeout") names no Service, but the address belongs to one: the
// virtual address of a Service, or the address of a pod it selects.
// Only addresses the model holds resolve; nothing is guessed.

// addressIndex maps the addresses of the Services and pods the model
// holds to their entities, built once per solve.
type addressIndex struct {
	services map[string]inventory.EntityID
	pods     map[string]inventory.EntityID
}

// addresses builds the index on first use.
func (v *view) addresses() *addressIndex {
	if v.addressIndex != nil {
		return v.addressIndex
	}
	index := &addressIndex{services: map[string]inventory.EntityID{},
		pods: map[string]inventory.EntityID{}}
	fill := func(kind inventory.Kind, attr string,
		into map[string]inventory.EntityID,
	) {
		for _, id := range v.s.Model.Entities(kind) {
			entity, ok := v.s.Model.Entity(id)
			if !ok {
				continue
			}
			if ip := net.ParseIP(attrText(entity, attr)); ip != nil {
				into[ip.String()] = id
			}
		}
	}
	fill(kube.KindService, kube.AttrClusterIP, index.services)
	fill(kube.KindPod, kube.AttrPodIP, index.pods)
	v.addressIndex = index
	return index
}

// serviceAtAddress resolves the host of a failed call, when it is an
// IP address, to the existing Service it reaches: the Service whose
// virtual address it is, else the Service selecting the pod that has
// the address (the one that serves the dialed port, the first by name
// otherwise).
func (v *view) serviceAtAddress(
	host string, port int, caller inventory.EntityID,
) (inventory.EntityID, bool) {
	ip := net.ParseIP(host)
	if ip == nil {
		return inventory.EntityID{}, false
	}
	index := v.addresses()
	if service, ok := index.services[ip.String()]; ok {
		v.gate(caller, service)
		return service, true
	}
	pod, ok := index.pods[ip.String()]
	if !ok || pod == caller {
		return inventory.EntityID{}, false
	}
	v.gate(caller, pod)
	services := v.servicesSelecting(pod)
	sort.Slice(services, func(i, j int) bool {
		return services[i].String() < services[j].String()
	})
	for _, service := range services {
		if v.servesPort(service, port) {
			v.gate(caller, service)
			return service, true
		}
	}
	return inventory.EntityID{}, false
}

// servesPort reports whether the Service exposes the port, or whether
// no port was dialed.
func (v *view) servesPort(service inventory.EntityID, port int) bool {
	if port == 0 {
		return true
	}
	entity, ok := v.s.Model.Entity(service)
	if !ok {
		return false
	}
	for _, entry := range strings.Split(attrText(entity, kube.AttrPorts),
		",") {
		published, rest, _ := strings.Cut(entry, "/")
		_, target, _ := strings.Cut(rest, "->")
		if published == strconv.Itoa(port) || target == strconv.Itoa(port) {
			return true
		}
	}
	return false
}
