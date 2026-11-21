package kube

import "testing"

func TestClusterServiceNameReadsAPodsStableName(t *testing.T) {
	for host, want := range map[string]string{
		"queue-0.queue.streaming.svc.cluster.local": "streaming/queue",
		"queue-0.queue.streaming.svc":               "streaming/queue",
		"queue.streaming.svc.cluster.local":         "streaming/queue",
		"queue.streaming":                           "streaming/queue",
	} {
		ns, name, ok := ClusterServiceName(host, "other")
		if !ok || ns+"/"+name != want {
			t.Errorf("%s: got %s/%s ok=%v, want %s", host, ns, name, ok,
				want)
		}
	}
	if _, _, ok := ClusterServiceName("a.b.c.example.com", "x"); ok {
		t.Error("an outside host is not a Service")
	}
}
