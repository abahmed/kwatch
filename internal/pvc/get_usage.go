package pvc

import (
	"context"
	"encoding/json"

	"github.com/abahmed/kwatch/internal/k8s"
)

type SummaryResponse struct {
	Pods []*Pod `json:"pods"`
}

type Pod struct {
	PodRef *Ref      `json:"podRef"`
	Volume []*Volume `json:"volume"`
}

type Volume struct {
	UsedBytes     int64  `json:"usedBytes"`
	CapacityBytes int64  `json:"capacityBytes"`
	Name          string `json:"name"`
	PvcRef        *Ref   `json:"pvcRef"`
}

type Ref struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// getNodeUsage gets list of pvc usage for specific node
func (p *PvcMonitor) getNodeUsage(ctx context.Context, nodeName string, pvByPVC map[string]string) ([]*PvcUsage, error) {
	result := make([]*PvcUsage, 0)

	summaryResponse, err := k8s.GetNodeSummary(ctx, p.client, nodeName)
	if err != nil {
		return result, err

	}

	var summaryObj SummaryResponse
	err = json.Unmarshal(summaryResponse, &summaryObj)
	if err != nil {
		return result, err
	}

	for _, pod := range summaryObj.Pods {
		if pod.PodRef == nil {
			continue
		}
		for _, vol := range pod.Volume {
			if vol.PvcRef == nil || len(vol.PvcRef.Name) == 0 {
				continue
			}
			if vol.CapacityBytes <= 0 {
				continue
			}

			pvName := pvByPVC[pod.PodRef.Namespace+"/"+vol.PvcRef.Name]
			if pvName == "" {
				continue
			}

			percentage :=
				(float64(vol.UsedBytes) / float64(vol.CapacityBytes)) * 100.0

			result = append(result, &PvcUsage{
				Name:            vol.PvcRef.Name,
				PVName:          pvName,
				Namespace:       pod.PodRef.Namespace,
				PodName:         pod.PodRef.Name,
				UsagePercentage: percentage,
			})
		}
	}

	return result, nil
}
