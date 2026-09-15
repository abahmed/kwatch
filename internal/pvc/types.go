package pvc

type PvcUsage struct {
	Name            string
	PVName          string
	Namespace       string
	PodName         string
	UsagePercentage float64
}

// key identifies usage by claim rather than bound PersistentVolume. The
// incident, silence, status, and insight paths all use namespace/claim.
func (u *PvcUsage) key() string {
	return u.Namespace + "/" + u.Name
}
