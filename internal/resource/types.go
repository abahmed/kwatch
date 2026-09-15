package resource

import (
	"time"

	"k8s.io/client-go/kubernetes"
	corev1lister "k8s.io/client-go/listers/core/v1"
)

type Config struct {
	CpuWarning                float64
	CpuCritical               float64
	MemWarning                float64
	MemCritical               float64
	FilesystemWarningPercent  float64
	FilesystemCriticalPercent float64
	InodeWarningPercent       float64
	InodeCriticalPercent      float64
	Interval                  time.Duration
	Client                    kubernetes.Interface
}

type Monitor struct {
	cfg        Config
	nodeLister corev1lister.NodeLister
	podLister  corev1lister.PodLister
	client     kubernetes.Interface
	interval   time.Duration
}

const defaultCheckInterval = 5 * time.Minute

type filesystemSummary struct {
	Node struct {
		FS *filesystemStats `json:"fs"`
	} `json:"node"`
}

type filesystemStats struct {
	CapacityBytes *uint64 `json:"capacityBytes"`
	UsedBytes     *uint64 `json:"usedBytes"`
	Inodes        *uint64 `json:"inodes"`
	InodesFree    *uint64 `json:"inodesFree"`
}
