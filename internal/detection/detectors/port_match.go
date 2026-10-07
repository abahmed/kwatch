package detectors

import (
	"strconv"
	"strings"
)

// servicePort is one port of a Service and where it leads.
type servicePort struct {
	number string
	name   string
	target string
}

// parseServicePorts reads kube.AttrServicePortSpecs.
func parseServicePorts(spec string) []servicePort {
	var out []servicePort
	for _, entry := range strings.Split(spec, ",") {
		fields := strings.Split(entry, ":")
		if len(fields) != 3 {
			continue
		}
		out = append(out, servicePort{
			number: fields[0], name: fields[1], target: fields[2]})
	}
	return out
}

// has reports whether the Service has a port that the backend port
// (a number or a name) refers to.
func hasServicePort(ports []servicePort, want string) bool {
	for _, port := range ports {
		if port.number == want || (port.name != "" && port.name == want) {
			return true
		}
	}
	return false
}

// describePorts writes the Service's ports the way people name them:
// "80/http, 9000".
func describePorts(ports []servicePort) string {
	parts := make([]string, 0, len(ports))
	for _, port := range ports {
		part := port.number
		if port.name != "" {
			part += "/" + port.name
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

// podPort is a port a pod declares.
type podPort struct {
	name   string
	number string
}

// parsePodPorts reads kube.AttrPodPorts ("http=8080,9090").
func parsePodPorts(spec string) []podPort {
	var out []podPort
	for _, entry := range strings.Split(spec, ",") {
		if entry == "" {
			continue
		}
		name, number, named := strings.Cut(entry, "=")
		if !named {
			name, number = "", entry
		}
		out = append(out, podPort{name: name, number: number})
	}
	return out
}

// isPortNumber reports whether a Service target is a number.
func isPortNumber(target string) bool {
	_, err := strconv.Atoi(target)
	return err == nil
}

// describePodPorts writes declared ports as "80 (http), 9090".
func describePodPorts(ports []podPort) string {
	seen := map[string]bool{}
	var parts []string
	for _, port := range ports {
		part := port.number
		if port.name != "" {
			part += " (" + port.name + ")"
		}
		if !seen[part] {
			seen[part] = true
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}
