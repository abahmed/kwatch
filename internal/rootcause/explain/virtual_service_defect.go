package explain

import (
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Defects of a Service that its callers see as plain failures: its own
// declaration cannot carry the call. Each is read from the Service and
// the call the failing pod's error names, never from the error's words.
const (
	// ModeProtocolMismatch is a Service that exposes the dialed port
	// over UDP only, called over TCP.
	ModeProtocolMismatch detection.Mode = "ProtocolMismatch"
	// ModeNameIsAddress is an ExternalName Service whose target is an
	// IP address, which a DNS alias cannot hold.
	ModeNameIsAddress detection.Mode = "NameIsAddress"
	// ModeFamilyMismatch is a Service whose virtual address is of an IP
	// family its pods have no address of.
	ModeFamilyMismatch detection.Mode = "FamilyMismatch"
)

// serviceDefectRows say that a Service whose declaration cannot carry
// the call explains the pods that fail calling it.
var serviceDefectRows = []Row{
	serviceDefectRow(RuleServiceProtocolMismatch, ModeProtocolMismatch),
	serviceDefectRow(RuleServiceNameIsAddress, ModeNameIsAddress),
	serviceDefectRow(RuleServiceFamilyMismatch, ModeFamilyMismatch),
}

func serviceDefectRow(name string, mode detection.Mode) Row {
	return Row{
		Name:   name,
		Cause:  Side{Kind: kube.KindService, Modes: []detection.Mode{mode}},
		Link:   LinkCalls,
		Effect: Side{Kind: kube.KindPod, Modes: callerFailures},
		Prior:  0.8,
	}
}

// tcpCall matches the words of a call made over TCP.
var tcpCall = regexp.MustCompile(`\btcp[46]?\b|https?://`)

// serviceDefectModes is the pseudo mode of the Service the effect's
// error names when the Service itself cannot carry the call.
func (v *view) serviceDefectModes(
	id, effect inventory.EntityID, link LinkType,
) []modeHealth {
	if link != LinkCalls {
		return nil
	}
	call, ok := v.clusterCallOf(effect)
	if !ok || call.service != id {
		return nil
	}
	e, found := v.s.Model.Entity(id)
	if !found {
		return nil
	}
	switch {
	case v.udpOnly(e, call):
		return pseudoFailing(ModeProtocolMismatch)
	case externalNameIsAddress(e):
		return pseudoFailing(ModeNameIsAddress)
	case v.familyMismatch(id, e):
		return pseudoFailing(ModeFamilyMismatch)
	}
	return nil
}

// udpOnly reports a Service that exposes the dialed port over UDP only
// while the call is over TCP.
func (v *view) udpOnly(e inventory.Entity, call clusterCall) bool {
	if call.port == 0 || !tcpCall.MatchString(strings.ToLower(call.line)) {
		return false
	}
	udp := false
	for _, entry := range strings.Split(attrText(e, kube.AttrPorts), ",") {
		published, rest, _ := strings.Cut(entry, "/")
		if published != strconv.Itoa(call.port) {
			continue
		}
		proto, _, _ := strings.Cut(rest, "->")
		if proto != "UDP" {
			return false
		}
		udp = true
	}
	return udp
}

// externalNameIsAddress reports an ExternalName Service that names an
// IP address.
func externalNameIsAddress(e inventory.Entity) bool {
	return attrText(e, kube.AttrServiceType) == "ExternalName" &&
		net.ParseIP(attrText(e, kube.AttrExternalName)) != nil
}

// familyMismatch reports a Service with no ready endpoint whose virtual
// address is IPv6 while the pods it selects only have IPv4 addresses,
// or the other way round.
func (v *view) familyMismatch(
	id inventory.EntityID, e inventory.Entity,
) bool {
	ip := net.ParseIP(attrText(e, kube.AttrClusterIP))
	if ip == nil || !v.noReadyEndpoints(id) {
		return false
	}
	want6 := ip.To4() == nil
	pods := 0
	for _, owner := range v.backendOwners(id) {
		for _, pod := range v.podsBehind(owner) {
			pe, ok := v.s.Model.Entity(pod)
			if !ok {
				continue
			}
			podIP := net.ParseIP(attrText(pe, kube.AttrPodIP))
			if podIP == nil {
				continue
			}
			if (podIP.To4() == nil) == want6 {
				return false
			}
			pods++
		}
	}
	return pods > 0
}
