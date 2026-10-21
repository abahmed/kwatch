package kube

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestStatsHintDetectsCertificateWithoutIPSANs(t *testing.T) {
	noIPs := &x509.Certificate{DNSNames: []string{"node-1"}}
	withIP := &x509.Certificate{
		IPAddresses: []net.IP{net.ParseIP("10.0.0.9")},
	}
	wrap := func(err error) error {
		return fmt.Errorf("read node-1: %w", &url.Error{
			Op: "Get", URL: "https://10.0.0.5:10250/stats/summary",
			Err: err,
		})
	}
	cases := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"no IP SANs", wrap(x509.HostnameError{
			Certificate: noIPs, Host: "10.0.0.5"}), hintNoIPSANs},
		{"text only", errors.New("x509: cannot validate certificate " +
			"for 10.0.0.5 because it doesn't contain any IP SANs"),
			hintNoIPSANs},
		{"other IP SAN", wrap(x509.HostnameError{
			Certificate: withIP, Host: "10.0.0.5"}), ""},
		{"DNS host", wrap(x509.HostnameError{
			Certificate: noIPs, Host: "node-2"}), ""},
		{"unknown authority", wrap(x509.UnknownAuthorityError{}), ""},
		{"timeout", errors.New("dial tcp: i/o timeout"), ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statsHint(tc.err); got != tc.want {
				t.Fatalf("statsHint() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatsPollerLogsHintForCertificateWithoutIPSANs(t *testing.T) {
	var out bytes.Buffer
	klog.LogToStderr(false)
	klog.SetOutput(&out)
	t.Cleanup(func() {
		klog.Flush()
		klog.SetOutput(nil)
		klog.LogToStderr(true)
	})
	now := fixedNow()
	p := NewStatsPoller(StatsConfig{
		Kubelet: mapKubelet{failures: map[string]error{
			"a": x509.HostnameError{
				Certificate: &x509.Certificate{}, Host: "10.0.0.5"},
		}},
		Now:    func() time.Time { return now },
		Submit: func(context.Context, ...inventory.Observation) {},
		Nodes:  statsNodes("a"),
	})

	p.poll(context.Background())
	p.poll(context.Background()) // a node must fail two polls in a row
	klog.Flush()

	assert.Contains(t, out.String(), "kubelet stats unavailable")
	assert.Contains(t, out.String(), "serverTLSBootstrap")
}
