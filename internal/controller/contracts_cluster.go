package controller

type ResourceProcessor interface {
	ProcessResourceQuota(string, bool) error
	ProcessLimitRange(string, bool) error
	ProcessNamespace(string, bool) error
	ProcessLease(string, bool) error
}
