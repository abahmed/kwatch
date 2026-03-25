package store

import (
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/detector/common"
)

const PVCKeyPrefix = "pvc:"

type PVCState struct {
	store  *Store
	usages map[string]*PVCUsageRecord
	mu     sync.RWMutex
}

type PVCUsageRecord struct {
	Name        string
	Namespace   string
	Capacity    int64
	Used        int64
	Percent     float64
	LastChecked time.Time
	IsAttached  bool
	Node        string
}

func NewPVCState(store *Store) *PVCState {
	p := &PVCState{
		store:  store,
		usages: make(map[string]*PVCUsageRecord),
	}
	p.load()
	return p
}

func (p *PVCState) Name() string {
	return "PVCState"
}

func (p *PVCState) UpdateUsage(name, namespace string, capacity, used int64, isAttached bool, node string) {
	key := namespace + "/" + name
	p.mu.Lock()
	defer p.mu.Unlock()

	percent := float64(0)
	if capacity > 0 {
		percent = float64(used) / float64(capacity) * 100
	}

	p.usages[key] = &PVCUsageRecord{
		Name:        name,
		Namespace:   namespace,
		Capacity:    capacity,
		Used:        used,
		Percent:     percent,
		LastChecked: time.Now(),
		IsAttached:  isAttached,
		Node:        node,
	}

	p.save()
}

func (p *PVCState) GetUsage(name, namespace string) *PVCUsageRecord {
	key := namespace + "/" + name
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.usages[key]
}

func (p *PVCState) GetAllUsages() []*PVCUsageRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*PVCUsageRecord, 0, len(p.usages))
	for _, u := range p.usages {
		result = append(result, u)
	}
	return result
}

func (p *PVCState) GetUsagesAboveThreshold(threshold float64) []*PVCUsageRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*PVCUsageRecord, 0)
	for _, u := range p.usages {
		if u.Percent >= threshold {
			result = append(result, u)
		}
	}
	return result
}

func (p *PVCState) GetDetachedUsages() []*PVCUsageRecord {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make([]*PVCUsageRecord, 0)
	for _, u := range p.usages {
		if !u.IsAttached {
			result = append(result, u)
		}
	}
	return result
}

func (p *PVCState) load() {
	p.mu.Lock()
	defer p.mu.Unlock()

	var usages map[string]*PVCUsageRecord
	err := p.store.Read(PVCKeyPrefix+"usages", &usages)
	if err != nil || usages == nil {
		p.usages = make(map[string]*PVCUsageRecord)
		return
	}
	p.usages = usages
}

func (p *PVCState) save() {
	if p.store == nil {
		return
	}
	_ = p.store.Write(PVCKeyPrefix+"usages", p.usages)
}

func (p *PVCState) Cleanup() {
	p.mu.Lock()
	defer p.mu.Unlock()

	cutoff := time.Now().Add(24 * time.Hour)
	for key, u := range p.usages {
		if u.LastChecked.Before(cutoff) && !u.IsAttached {
			delete(p.usages, key)
		}
	}
	p.save()
}

func (p *PVCState) GetStats() PVCStateStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	attached := 0
	detached := 0
	aboveThreshold := 0

	for _, u := range p.usages {
		if u.IsAttached {
			attached++
		} else {
			detached++
		}
		if u.Percent >= 80 {
			aboveThreshold++
		}
	}

	return PVCStateStats{
		TotalPVCs:      len(p.usages),
		Attached:       attached,
		Detached:       detached,
		AboveThreshold: aboveThreshold,
	}
}

type PVCStateStats struct {
	TotalPVCs      int
	Attached       int
	Detached       int
	AboveThreshold int
}

func ConvertToCommonPVCUsage(record *PVCUsageRecord) common.PVCUsage {
	if record == nil {
		return common.PVCUsage{}
	}
	return common.PVCUsage{
		Name:        record.Name,
		Namespace:   record.Namespace,
		Capacity:    record.Capacity,
		Used:        record.Used,
		Percent:     record.Percent,
		LastChecked: record.LastChecked,
		IsAttached:  record.IsAttached,
	}
}
