package incident

import "github.com/abahmed/kwatch/internal/inventory"

// ReplicaCount is how many replicas a workload wants and how many of
// them are ready, as the model read at the time of a message.
type ReplicaCount struct {
	Desired int
	Ready   int
}

// ReplicaCountOf reads the replica counts of the workload p is rooted
// at. nil when the model is nil, the root is not a workload the model
// holds, or it has no replicas short, so a message never states a count
// that says nothing.
func ReplicaCountOf(model inventory.Reader, p Incident) *ReplicaCount {
	if model == nil {
		return nil
	}
	desired, ready, ok := ReplicasOf(model, p.Root)
	if !ok || desired <= 0 || ready >= desired {
		return nil
	}
	return &ReplicaCount{Desired: int(desired), Ready: int(ready)}
}
