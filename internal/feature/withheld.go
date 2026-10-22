package feature

import (
	"hash/fnv"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WithheldCategory groups the Pro findings a Community installation
// counts and never shows.
type WithheldCategory int

const (
	WithheldReadiness WithheldCategory = iota
	WithheldSharedCauses
	WithheldPreemption
	WithheldGates
	withheldCategories
)

// withheldLabels are the singular and plural words of each category, in
// the order the counts are shown.
var withheldLabels = [withheldCategories][2]string{
	{"readiness probe", "readiness probes"},
	{"shared cause", "shared causes"},
	{"preemption and admission", "preemption and admission"},
	{"scheduling gate", "scheduling gates"},
}

// WithheldDays is the window the counts cover.
const WithheldDays = 7

// withheldPerDay bounds the distinct findings remembered per day. A day
// that reaches it stops counting: the counts are a hint, not a ledger.
const withheldPerDay = 2048

// WithheldCounts are the withheld findings per category.
type WithheldCounts [withheldCategories]int

// Total is the sum of every category.
func (c WithheldCounts) Total() int {
	n := 0
	for _, v := range c {
		n += v
	}
	return n
}

// Text lists the non-zero categories: "4 readiness probes, 1 shared
// cause". It is empty when every count is zero.
func (c WithheldCounts) Text() string {
	var parts []string
	for i, n := range c {
		if n == 0 {
			continue
		}
		label := withheldLabels[i][1]
		if n == 1 {
			label = withheldLabels[i][0]
		}
		parts = append(parts, strconv.Itoa(n)+" "+label)
	}
	return strings.Join(parts, ", ")
}

// Withheld counts, over the last WithheldDays days, the distinct Pro
// findings a Community installation found and did not show. It keeps
// only hashes of what it counted, never names, and a bounded number of
// them. A nil Withheld counts nothing. It is safe for concurrent use.
type Withheld struct {
	mu   sync.Mutex
	days [WithheldDays]withheldDay
}

type withheldDay struct {
	day    int64
	seen   map[uint64]struct{}
	counts WithheldCounts
}

// NewWithheld returns an empty counter.
func NewWithheld() *Withheld { return &Withheld{} }

// Record counts the finding named by key in category at now, once per
// window.
func (w *Withheld) Record(
	now time.Time, category WithheldCategory, key string,
) {
	if w == nil || category < 0 || category >= withheldCategories {
		return
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.Itoa(int(category)) + "\x00" + key))
	sum := h.Sum64()
	today := dayOf(now)
	if today < 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	slot := &w.days[today%WithheldDays]
	if slot.day != today || slot.seen == nil {
		*slot = withheldDay{day: today, seen: map[uint64]struct{}{}}
	}
	for i := range w.days {
		d := &w.days[i]
		if _, ok := d.seen[sum]; ok && inWindow(d.day, today) {
			return
		}
	}
	if len(slot.seen) >= withheldPerDay {
		return
	}
	slot.seen[sum] = struct{}{}
	slot.counts[category]++
}

// Counts returns the counts of the window that ends at now.
func (w *Withheld) Counts(now time.Time) WithheldCounts {
	var out WithheldCounts
	if w == nil {
		return out
	}
	today := dayOf(now)
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, d := range w.days {
		if d.seen == nil || !inWindow(d.day, today) {
			continue
		}
		for i, n := range d.counts {
			out[i] += n
		}
	}
	return out
}

func dayOf(t time.Time) int64 {
	return t.UTC().Unix() / int64(24*time.Hour/time.Second)
}

func inWindow(day, today int64) bool {
	return day <= today && day > today-WithheldDays
}
