package pvcmonitor

import (
	"encoding/json"

	"github.com/abahmed/kwatch/util"
	"github.com/sirupsen/logrus"
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
func (p *PvcMonitor) getNodeUsage(nodeName string) ([]*PvcUsage, error) {
	result := make([]*PvcUsage, 0)

	summaryResponse, err := util.GetNodeSummary(p.client, nodeName)
	if err != nil {
		return result, err

	}

	var summaryObj SummaryResponse
	err = json.Unmarshal(summaryResponse, &summaryObj)
	if err != nil {
		return result, err
	}

	for _, pod := range summaryObj.Pods {
		for _, vol := range pod.Volume {
			if vol.PvcRef == nil || len(vol.PvcRef.Name) == 0 {
				continue
			}

			pvName, err :=
				util.GetPVNameFromPVC(
					p.client,
					pod.PodRef.Namespace,
					vol.PvcRef.Name)
			if err != nil {
				logrus.Errorf(
					"failed to get pv name for pvc %s: %s",
					vol.PvcRef.Name,
					err.Error())
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
