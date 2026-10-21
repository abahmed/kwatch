package kube

import (
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// AttrDriverNames lists the driver or provisioner names a pod's
// containers are given as literal arguments or environment values,
// lowercase and space-separated: "--driver-name=a.example.com" gives
// "a.example.com". A registrar is given the name inside a kubelet
// socket path instead (see pluginDirName); the name is kept, not the
// path. Absent when there are none.
const AttrDriverNames = "driver.names"

// DriverSocketSuffix ends the name of the socket a driver registers with
// the kubelet: "<driver>-reg.sock".
const DriverSocketSuffix = "-reg.sock"

// maxDriverNames bounds the names kept, and maxDriverNameLen the length
// of one.
const (
	maxDriverNames   = 8
	maxDriverNameLen = 128
)

// driverNames collects the values of flags and variables whose name says
// driver, provisioner or registration, and the driver directories of the
// pod's host paths. Only those are kept: other arguments may hold
// credentials.
func driverNames(pod *corev1.Pod) string {
	var out []string
	forEachContainer(pod, func(c corev1.Container, _ bool) {
		args := append(append([]string{}, c.Command...), c.Args...)
		for _, value := range flagValues(args) {
			out = addName(out, value)
		}
		for _, env := range c.Env {
			if namesDriver(env.Name) && env.ValueFrom == nil {
				out = addName(out, env.Value)
			}
		}
	})
	for _, volume := range pod.Spec.Volumes {
		if volume.HostPath != nil {
			out = addName(out, pluginDirName(volume.HostPath.Path))
		}
	}
	return strings.Join(out, " ")
}

// flagValues are the values of the driver or provisioner flags in args,
// written "--flag=value" or "--flag value".
func flagValues(args []string) []string {
	var out []string
	for i, arg := range args {
		flag, value, hasValue := strings.Cut(arg, "=")
		if !strings.HasPrefix(flag, "-") || !namesDriver(flag) {
			continue
		}
		if !hasValue && i+1 < len(args) &&
			!strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
		}
		out = append(out, value)
	}
	return out
}

// addName appends the driver name in value, lowercased, unless there is
// none, it is too long, has spaces, repeats a name, or the list is full.
func addName(names []string, value string) []string {
	value = driverName(value)
	if value == "" || len(value) > maxDriverNameLen ||
		strings.ContainsAny(value, " \t\n") ||
		len(names) >= maxDriverNames || slices.Contains(names, value) {
		return names
	}
	return append(names, value)
}

// driverName is the driver a flag or variable value gives: the value
// itself, or the driver's directory or registration socket in a path.
func driverName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if strings.Contains(value, "/") {
		return pluginDirName(value)
	}
	return strings.TrimSuffix(value, DriverSocketSuffix)
}

// pluginDirName is the driver a kubelet plugin path names: the segment
// after "plugins" or "plugins_registry", so that
// ".../plugins/a.example.com/csi.sock" gives "a.example.com" and
// ".../plugins_registry/a.example.com-reg.sock" does too. The kubelet's
// own "kubernetes.io" directory names no driver.
func pluginDirName(path string) string {
	segments := strings.Split(strings.ToLower(path), "/")
	for i, segment := range segments[:len(segments)-1] {
		if segment != "plugins" && segment != "plugins_registry" {
			continue
		}
		name := strings.TrimSuffix(segments[i+1], DriverSocketSuffix)
		if name == "kubernetes.io" {
			return ""
		}
		return name
	}
	return ""
}

// namesDriver reports a flag or variable name that says driver,
// provisioner or kubelet registration, and is not named like a
// credential.
func namesDriver(name string) bool {
	lower := strings.ToLower(name)
	return !secretNamed(lower) && (strings.Contains(lower, "driver") ||
		strings.Contains(lower, "provisioner") ||
		strings.Contains(lower, "registration"))
}
