package kube_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	ktesting "k8s.io/client-go/testing"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestLogReaderExcerptRejectsBadInput(t *testing.T) {
	id := kube.ContainerID(testNamespace, "p", "app")
	assert.Nil(t, kube.LogReader{}.Excerpt(context.Background(), id))
	r := kube.LogReader{Client: fake.NewSimpleClientset()}
	bad := id
	bad.Name = "noslash"
	assert.Nil(t, r.Excerpt(context.Background(), bad))
}

func TestLogReaderExcerptReadsFakeLogs(t *testing.T) {
	r := kube.LogReader{Client: fake.NewSimpleClientset(pod("p"))}
	id := kube.ContainerID(testNamespace, "p", "app")
	// The fake clientset serves the constant body "fake logs".
	got := r.Excerpt(context.Background(), id)
	assert.Equal(t, []string{"fake logs"}, got)
}

// serveLogs makes the fake API serve body as a container log, cut to
// the requested LimitBytes from its start as the API server does.
func serveLogs(client *fake.Clientset, body string) {
	client.PrependReactor("get", "pods",
		func(action ktesting.Action) (bool, runtime.Object, error) {
			generic, ok := action.(ktesting.GenericAction)
			if !ok || action.GetSubresource() != "log" {
				return false, nil, nil
			}
			out := body
			opts, _ := generic.GetValue().(*corev1.PodLogOptions)
			if opts != nil && opts.LimitBytes != nil &&
				int64(len(out)) > *opts.LimitBytes {
				out = out[:*opts.LimitBytes]
			}
			return true, &runtime.Unknown{Raw: []byte(out)}, nil
		})
}

// A tail longer than the kept bytes still ends with the crash message:
// the server would cut a 64KiB limit off the end, so the reader asks
// for more and keeps the end itself, from a whole line.
func TestLogReaderKeepsTheEndOfALongTail(t *testing.T) {
	noise := strings.Repeat("x", 700)
	var b strings.Builder
	for i := 0; i < 199; i++ {
		b.WriteString("chatter " + noise + "\n")
	}
	b.WriteString("panic: boom\n")
	require.Greater(t, b.Len(), 64<<10)
	client := fake.NewSimpleClientset(pod("p"))
	serveLogs(client, b.String())
	r := kube.LogReader{Client: client}

	lines := r.Lines(context.Background(),
		kube.ContainerID(testNamespace, "p", "app"))

	require.NotEmpty(t, lines)
	assert.Equal(t, "panic: boom", lines[len(lines)-1])
	assert.True(t, strings.HasPrefix(lines[0], "chatter "),
		"the first kept line is whole")
	assert.Less(t, len(lines), 199, "the front of the tail is dropped")
}
