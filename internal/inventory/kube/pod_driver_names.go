package kube

import (
	"slices"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// AttrDriverNames lists the driver or provisioner names a pod's
// containers are given as literal arguments or environment values,
// lowercase and space-separated: "--driver-name=a.example.com" gives
// "a.example.com". Absent when there are none.
const AttrDriverNames = "driver.names"

// maxDriverNames bounds the names kept, and maxDriverNameLen the length
// of one.
const (
	maxDriverNames   = 8
	maxDriverNameLen = 128
)

// driverNames collects the values of flags and variables whose name says
// driver or provisioner. Only those are kept: other arguments may hold
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

// addName appends value, lowercased, unless it is empty, too long, has
// spaces, repeats a name, or the list is full.
func addName(names []string, value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || len(value) > maxDriverNameLen ||
		strings.ContainsAny(value, " \t\n") ||
		len(names) >= maxDriverNames || slices.Contains(names, value) {
		return names
	}
	return append(names, value)
}

// namesDriver reports a flag or variable name that says driver or
// provisioner.
func namesDriver(name string) bool {
	name = strings.ToLower(name)
	return strings.Contains(name, "driver") ||
		strings.Contains(name, "provisioner")
}
