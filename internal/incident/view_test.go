package incident

import (
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A read of the published view never waits for the manager's lock: a
// long Apply does not hold up the dashboard.
func TestViewReadDoesNotWaitForTheManager(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	r.m.Publish()

	r.m.mu.Lock() // a long Apply or Tick
	got := make(chan *View, 1)
	go func() { got <- r.m.View() }()
	select {
	case v := <-got:
		require.NotNil(t, v)
		assert.Len(t, v.Incidents(), 1)
	case <-time.After(5 * time.Second):
		t.Fatal("a view read waited for the manager lock")
	}
	r.m.mu.Unlock()
}

// A view stays what it was when it was published; the next one is a
// new revision with the change.
func TestViewIsOneRevisionThatNeverChanges(t *testing.T) {
	r := newRig(t, Config{})
	assert.Nil(t, r.m.View(), "nothing before the first publish")
	r.raise(at(0), podSig("web"))
	first := r.m.Publish()

	r.raise(at(time.Second), podSig("api"))
	assert.Len(t, first.Incidents(), 1)
	assert.Same(t, first, r.m.View())

	second := r.m.Publish()
	assert.Equal(t, first.Revision+1, second.Revision)
	assert.Len(t, second.Incidents(), 2)
	id := second.Incidents()[0].ID
	require.NotNil(t, second.Incident(id))
	assert.Equal(t, id, second.Incident(id).ID)
	assert.Nil(t, second.Incident("missing"))
}

// The view's old problems leave out the objects that are issues in
// the same view.
func TestViewListsAnObjectOnce(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	old := oldSig("report", reasons.JobFailed, detection.Info,
		600*24*time.Hour)
	r.raise(at(0), old)
	assert.Len(t, r.m.Publish().OldProblems(), 1)
	assert.Equal(t, 30, r.m.View().OldProblemDays())

	young := sig(entity(kube.KindJob, "report"), reasons.CrashLoopBackOff,
		detection.Warning)
	young.Since = at(time.Minute)
	r.raise(at(time.Minute), young)
	v := r.m.Publish()
	assert.Len(t, v.Incidents(), 1)
	assert.Empty(t, v.OldProblems())
}

// Readers on other goroutines read views while the loop applies, ticks
// and publishes (run with -race).
func TestViewReadsAreSafeDuringTheLoop(t *testing.T) {
	r := newRig(t, Config{})
	r.m.Publish()
	done := make(chan struct{})
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
				}
				v := r.m.View()
				for _, p := range v.Incidents() {
					_ = len(p.Members) + len(p.Timeline)
					_ = v.Incident(p.ID)
				}
			}
		}()
	}
	for i := range 50 {
		r.raise(at(time.Duration(i)*time.Second),
			podSig("web-"+strconv.Itoa(i%5)))
		r.tick(at(time.Duration(i) * time.Second))
		r.m.Publish()
	}
	close(done)
	wg.Wait()
}

// manyIncidents is a manager holding n open incidents, published.
func manyIncidents(b *testing.B, n int) *rig {
	b.Helper()
	r := newRig(b, Config{})
	sigs := make([]detection.Finding, 0, n)
	for i := range n {
		sigs = append(sigs, podSig("web-"+strconv.Itoa(i)))
	}
	r.raise(at(0), sigs...)
	r.tick(at(DefaultSettle))
	r.m.Publish()
	return r
}

// BenchmarkDashboardReadCopies is the read every dashboard request
// made before views: a deep copy of every incident under the lock.
func BenchmarkDashboardReadCopies(b *testing.B) {
	r := manyIncidents(b, 2000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = r.m.Incidents()
	}
}

// BenchmarkDashboardReadView is the read a request makes now.
func BenchmarkDashboardReadView(b *testing.B) {
	r := manyIncidents(b, 2000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = r.m.View().Incidents()
	}
}

// BenchmarkPublish is what the loop pays once per iteration instead.
func BenchmarkPublish(b *testing.B) {
	r := manyIncidents(b, 2000)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		r.m.Publish()
	}
}
