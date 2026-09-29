package kube

// statsSummary is the subset of the kubelet summary API kwatch reads.
type statsSummary struct {
	Node struct {
		FS      fsStats      `json:"fs"`
		Network networkStats `json:"network"`
		Memory  resourceSet  `json:"memory"`
		CPU     resourceSet  `json:"cpu"`
		IO      resourceSet  `json:"io"`
	} `json:"node"`
	Pods []struct {
		PodRef struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"podRef"`
		Containers []struct {
			Name string `json:"name"`
			CPU  struct {
				UsageNanoCores *uint64 `json:"usageNanoCores"`
			} `json:"cpu"`
			Memory struct {
				WorkingSetBytes *uint64 `json:"workingSetBytes"`
			} `json:"memory"`
		} `json:"containers"`
		EphemeralStorage *fsStats `json:"ephemeral-storage"`
		Volumes          []struct {
			fsStats
			PVCRef *struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"pvcRef"`
		} `json:"volume"`
	} `json:"pods"`
}

type fsStats struct {
	CapacityBytes uint64 `json:"capacityBytes"`
	UsedBytes     uint64 `json:"usedBytes"`
	Inodes        uint64 `json:"inodes"`
	InodesFree    uint64 `json:"inodesFree"`
}

// resourceSet carries pressure stall information, available when the
// kubelet exposes PSI.
type resourceSet struct {
	PSI *psiStats `json:"psi"`
}

type psiStats struct {
	Some struct {
		Avg60 float64 `json:"avg60"`
	} `json:"some"`
}

type volumeUsage struct {
	Namespace     string
	Name          string
	UsedBytes     uint64
	CapacityBytes uint64
}

// volumes lists persistent volume usage, once per claim.
func (s statsSummary) volumes() []volumeUsage {
	seen := map[string]bool{}
	var out []volumeUsage
	for _, pod := range s.Pods {
		for _, v := range pod.Volumes {
			if v.PVCRef == nil {
				continue
			}
			key := v.PVCRef.Namespace + "/" + v.PVCRef.Name
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, volumeUsage{
				Namespace: v.PVCRef.Namespace, Name: v.PVCRef.Name,
				UsedBytes: v.UsedBytes, CapacityBytes: v.CapacityBytes,
			})
		}
	}
	return out
}

type networkStats struct {
	Interfaces []struct {
		RxErrors *uint64 `json:"rxErrors"`
		TxErrors *uint64 `json:"txErrors"`
	} `json:"interfaces"`
}

// errors sums receive and transmit errors over all interfaces.
func (n networkStats) errors() (uint64, bool) {
	var total uint64
	found := false
	for _, i := range n.Interfaces {
		if i.RxErrors != nil {
			total += *i.RxErrors
			found = true
		}
		if i.TxErrors != nil {
			total += *i.TxErrors
			found = true
		}
	}
	return total, found
}
