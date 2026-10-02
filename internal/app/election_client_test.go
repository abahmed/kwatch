package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/abahmed/kwatch/internal/kubeclient"
)

func TestElectionClientPrefersElectionClient(t *testing.T) {
	electionClientSet := fake.NewSimpleClientset()
	kubeClientSet := fake.NewSimpleClientset()

	clients := kubeclient.ClientSet{
		Election:   electionClientSet,
		Kubernetes: kubeClientSet,
	}

	result := electionClient(clients)
	assert.Equal(t, electionClientSet, result)
}

func TestElectionClientFallsBackToKubernetesClient(t *testing.T) {
	kubeClientSet := fake.NewSimpleClientset()

	clients := kubeclient.ClientSet{
		Election:   nil,
		Kubernetes: kubeClientSet,
	}

	result := electionClient(clients)
	assert.Equal(t, kubeClientSet, result)
}

func TestElectionClientBothAvailable(t *testing.T) {
	electionClientSet := fake.NewSimpleClientset()
	kubeClientSet := fake.NewSimpleClientset()

	clients := kubeclient.ClientSet{
		Election:   electionClientSet,
		Kubernetes: kubeClientSet,
	}

	result := electionClient(clients)
	assert.NotNil(t, result)
	assert.Same(t, electionClientSet, result)
}
