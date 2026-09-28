package resource

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

func TestMonitorCheckFilesystemSkipsUnreadableNodes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.Contains(r.URL.Path, "/nodes/full/"):
				_, _ = w.Write([]byte(`{"node":{"fs":{"capacityBytes":100,` +
					`"usedBytes":99,"inodes":100,"inodesFree":90}}}`))
			case strings.Contains(r.URL.Path, "/nodes/ok/"):
				_, _ = w.Write([]byte(`{"node":{"fs":{"capacityBytes":100,` +
					`"usedBytes":10,"inodes":100,"inodesFree":90}}}`))
			default:
				w.WriteHeader(http.StatusServiceUnavailable)
			}
		},
	))
	defer server.Close()
	client, err := kubernetes.NewForConfig(&rest.Config{Host: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	indexer := cache.NewIndexer(
		cache.MetaNamespaceKeyFunc, cache.Indexers{},
	)
	for _, name := range []string{"full", "down", "ok"} {
		if err := indexer.Add(&corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: name},
		}); err != nil {
			t.Fatal(err)
		}
	}
	monitor := NewMonitor(Config{
		FilesystemWarningPercent: 80, FilesystemCriticalPercent: 95,
		Client: client,
	}, corev1lister.NewNodeLister(indexer), nil)

	signals, evaluated := monitor.checkFilesystem(context.Background())

	sort.Strings(evaluated)
	if strings.Join(evaluated, ",") != "full,ok" {
		t.Fatalf("evaluated = %v, want full,ok", evaluated)
	}
	if len(signals) != 1 || signals[0].NodeName != "full" {
		t.Fatalf("signals = %+v, want one for node full", signals)
	}
}
