package kube

import (
	"crypto/x509"
	"errors"
	"net"
	"strings"
)

// hintNoIPSANs is the log hint for a kubelet serving certificate without
// IP SANs, the kubeadm default. TLS verification stays on; the fix is a
// kubelet certificate that names the node IP.
const hintNoIPSANs = "the kubelet serving certificate has no IP SANs; " +
	"enable serverTLSBootstrap in the kubelet config and approve the " +
	"kubelet-serving CSRs"

// statsHint returns a short actionable hint for a kubelet read error,
// or "" when kwatch has nothing more useful to say than the error.
func statsHint(err error) string {
	if err == nil {
		return ""
	}
	if lacksIPSANs(err) {
		return hintNoIPSANs
	}
	return ""
}

// lacksIPSANs reports whether err is a certificate check that failed
// because kwatch dialled an IP the certificate does not list and the
// certificate lists no IP at all.
func lacksIPSANs(err error) bool {
	var host x509.HostnameError
	if errors.As(err, &host) && host.Certificate != nil {
		return net.ParseIP(host.Host) != nil &&
			len(host.Certificate.IPAddresses) == 0
	}
	return strings.Contains(err.Error(), "doesn't contain any IP SANs")
}
