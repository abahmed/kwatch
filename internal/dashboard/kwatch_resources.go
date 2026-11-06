package dashboard

import "github.com/abahmed/kwatch/internal/health"

// resourceSource reads kwatch's own memory, CPU and disk use.
type resourceSource func() health.ResourceUse

// SetResources sets where /api/kwatch reads kwatch's own use; nil
// leaves it out.
func (s *Server) SetResources(src func() health.ResourceUse) {
	if src == nil {
		s.sources.resources.Store(nil)
		return
	}
	v := resourceSource(src)
	s.sources.resources.Store(&v)
}

// ResourcesStatus is kwatch's own use against its container's limits.
// A part is absent when it can't be read; a limit of 0 (left out)
// means none is set. StateFile repeats the state file's size and cap
// beside the disk it lives on.
type ResourcesStatus struct {
	Memory    *MemoryUse       `json:"memory,omitempty"`
	CPU       *CPUUse          `json:"cpu,omitempty"`
	Disk      *DiskUse         `json:"disk,omitempty"`
	StateFile *StateFileStatus `json:"stateFile,omitempty"`
}

// MemoryUse is the bytes in use and the memory limit.
type MemoryUse struct {
	Used  int64 `json:"used"`
	Limit int64 `json:"limit,omitempty"`
}

// CPUUse is the cores in use (over the last sample) and the limit.
type CPUUse struct {
	Cores      float64 `json:"cores"`
	LimitCores float64 `json:"limitCores,omitempty"`
}

// DiskUse is the space used and the size of the state file's volume.
type DiskUse struct {
	Used  int64 `json:"used"`
	Total int64 `json:"total"`
}

// resources is the Resources part of /api/kwatch, or nil when nothing
// is known.
func (s *Server) resources(file *StateFileStatus) *ResourcesStatus {
	src := s.sources.resources.Load()
	if src == nil {
		return nil
	}
	u := (*src)()
	out := &ResourcesStatus{}
	if u.MemoryKnown {
		out.Memory = &MemoryUse{Used: u.MemoryBytes, Limit: u.MemoryLimit}
	}
	if u.CPUKnown {
		out.CPU = &CPUUse{Cores: u.CPUCores, LimitCores: u.CPULimit}
	}
	if u.DiskKnown && file != nil {
		out.Disk = &DiskUse{Used: u.DiskUsed, Total: u.DiskTotal}
		out.StateFile = file
	}
	if out.Memory == nil && out.CPU == nil && out.Disk == nil {
		return nil
	}
	return out
}
