package kube

import (
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// AttrQuotaNearLimit lists the quota resources used to at least
// QuotaNearLimitRatio of their hard limit but not yet exhausted, as
// "name=percent%" sorted by name.
const AttrQuotaNearLimit = "quota.near.limit"

// AttrQuotaZeroHard lists the quota resources whose hard limit is zero,
// comma separated. A zero limit is a deliberate "none allowed" (a namespace
// that may not use a resource), not a quota that ran out; such resources
// are also named in AttrExhausted, so a detector skips them using this.
const AttrQuotaZeroHard = "quota.zero.hard"

// QuotaNearLimitRatio is the share of a hard limit that counts as near.
const QuotaNearLimitRatio = 0.8

func quotaNearLimit(status corev1.ResourceQuotaStatus) string {
	var near []string
	for name, hard := range status.Hard {
		used, ok := status.Used[name]
		limit := hard.AsApproximateFloat64()
		if !ok || limit <= 0 {
			continue
		}
		ratio := used.AsApproximateFloat64() / limit
		if ratio < QuotaNearLimitRatio || used.Cmp(hard) >= 0 {
			continue
		}
		percent := strconv.Itoa(int(ratio * 100))
		near = append(near, string(name)+"="+percent+"%")
	}
	sort.Strings(near)
	return strings.Join(near, ",")
}
