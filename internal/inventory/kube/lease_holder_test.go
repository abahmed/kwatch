package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	coordinationv1 "k8s.io/api/coordination/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLeaseAttributesCarryAcquireTimeWhenSet(t *testing.T) {
	holder := "operator_1"
	at := metav1.NewMicroTime(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	lease := coordinationv1.Lease{Spec: coordinationv1.LeaseSpec{
		HolderIdentity: &holder, RenewTime: &at,
	}}
	assert.NotContains(t, leaseAttributes(lease, 15), AttrLeaseAcquired)

	lease.Spec.AcquireTime = &at
	got := leaseAttributes(lease, 15)[AttrLeaseAcquired]
	assert.True(t, at.Time.Equal(got.AsTime()))
}
