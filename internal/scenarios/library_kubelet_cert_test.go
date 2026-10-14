package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
)

// kubeletCertScenarios are nodes whose kubelet reports a certificate
// error as the reason it is not ready.
func kubeletCertScenarios() []scenario {
	return []scenario{kubeletCertExpired()}
}

// kubeletCertExpired: n1's Ready condition goes False and its own
// message is an x509 error. The node message quotes it as what the
// kubelet reports; kwatch does not guess when it expires.
func kubeletCertExpired() scenario {
	return scenario{
		expect: expectation{
			Name: "kubelet-cert-expired",
			Description: "A node is not ready and its kubelet reports " +
				"an x509 certificate error.",
			Root: "node//n1", Tier: "page", MaxMessages: 3,
			Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			n1 := c.node("n1", "zone-a")
			c.list(n1, c.node("n2", "zone-a"), c.node("n3", "zone-a"))
			c.after(10 * time.Minute)
			down := last(c, n1)
			setNodeCondition(down, corev1.NodeReady,
				corev1.ConditionFalse, "KubeletNotReady",
				"failed to renew client certificate: Post "+
					"\"https://10.0.0.1:6443/apis/certificates.k8s.io/"+
					"v1/certificatesigningrequests\": x509: certificate "+
					"has expired or is not yet valid", c.now)
			c.update(down)
			c.after(15 * time.Minute)
		},
	}
}
