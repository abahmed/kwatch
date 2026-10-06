package delivery

// offerResult is what offering a job to a provider queue did.
type offerResult struct {
	// accepted is false when the queue had no room for the job.
	accepted bool
	// superseded is a queued job of the same conversation that the new
	// one replaced; everything it said, the new one says too.
	superseded *deliverJob
	// displaced is a queued job pushed out to make room for a resolve.
	displaced *deliverJob
}

// offerQueuedJob queues a job, coalescing without growing the queue. The
// manager lock serializes producers; the worker may still consume jobs.
//
// A queued, unsent job of the same conversation is always replaced, so a
// provider recovering from an outage sends each conversation's newest
// state once instead of every revision in turn. A resolve is never
// dropped for a routine job: in a full queue with nothing to replace it
// takes the place of the newest routine job, which goes to the overflow
// summary instead. It never takes the place of another resolve or of a
// page announcement, whose alert would then never open; when only those
// are queued the resolve is dropped and logged as lost.
func offerQueuedJob(
	queue chan deliverJob,
	job deliverJob,
) offerResult {
	if job.kind != jobIncident || job.incident == nil {
		select {
		case queue <- job:
			return offerResult{accepted: true}
		default:
		}
	}
	queued := drainQueuedJobs(queue)
	queued, result := coalesce(queued, job, cap(queue))
	if result.accepted && job.urgent() {
		queued = moveAheadOfRoutine(queued, job)
	}
	refillQueuedJobs(queue, queued)
	return result
}

// urgent reports whether the job must not wait behind routine ones: a
// resolve closes an alert someone may be looking at, and a page wakes
// someone up. Every other job is paced at one every few seconds, so a
// full queue of them would hold an urgent job for minutes.
func (j deliverJob) urgent() bool {
	return j.kind == jobIncident && j.incident != nil &&
		(j.incident.Resolved() || j.incident.IsPage())
}

// moveAheadOfRoutine moves the job to the end of the urgent jobs at the
// front of the queue, ahead of every routine one. Urgent jobs keep their
// order among themselves, and a conversation's own messages never swap:
// the arriving job either replaced the conversation's queued message or
// is the only queued message of it.
func moveAheadOfRoutine(queued []deliverJob, job deliverJob) []deliverJob {
	at := -1
	for i, q := range queued {
		if q.outboxID == job.outboxID && q.target == job.target &&
			q.key() == job.key() {
			at = i
		}
	}
	if at < 0 {
		return queued
	}
	front := 0
	for front < at && queued[front].urgent() {
		front++
	}
	moved := queued[at]
	copy(queued[front+1:at+1], queued[front:at])
	queued[front] = moved
	return queued
}

// coalesce applies the full-queue rules to the drained jobs and returns
// the jobs to put back.
func coalesce(
	queued []deliverJob, job deliverJob, capacity int,
) ([]deliverJob, offerResult) {
	if index := replacementIndex(queued, job); index >= 0 {
		// The newer message keeps the conversation's queue position.
		old := queued[index]
		queued[index] = job
		return queued, offerResult{accepted: true, superseded: &old}
	}
	if len(queued) < capacity {
		return append(queued, job), offerResult{accepted: true}
	}
	if !job.isResolve() {
		return queued, offerResult{}
	}
	index := displacementVictim(queued)
	if index < 0 {
		return queued, offerResult{}
	}
	old := queued[index]
	queued = append(queued[:index], queued[index+1:]...)
	return append(queued, job), offerResult{accepted: true, displaced: &old}
}

func drainQueuedJobs(queue chan deliverJob) []deliverJob {
	jobs := make([]deliverJob, 0, len(queue))
	for {
		select {
		case job, open := <-queue:
			if !open {
				return jobs
			}
			jobs = append(jobs, job)
		default:
			return jobs
		}
	}
}

func refillQueuedJobs(queue chan deliverJob, jobs []deliverJob) {
	for _, job := range jobs {
		queue <- job
	}
}

// replacementIndex finds a queued incident of the same conversation and
// provider that the arriving one may replace. A queued resolve is only
// replaced by another resolve, so coalescing never drops a resolve.
func replacementIndex(queued []deliverJob, arriving deliverJob) int {
	if arriving.kind != jobIncident || arriving.incident == nil {
		return -1
	}
	for i := len(queued) - 1; i >= 0; i-- {
		old := queued[i]
		if old.kind != jobIncident || old.incident == nil ||
			old.incident.Key != arriving.incident.Key ||
			old.target != arriving.target {
			continue
		}
		if old.isResolve() && !arriving.isResolve() {
			return -1
		}
		return i
	}
	return -1
}

// displacementVictim picks the queued job a full queue gives up for a
// resolve: the newest routine job, else the newest other job that is
// neither a resolve nor a page announcement. It is -1 when there is none.
func displacementVictim(queued []deliverJob) int {
	fallback := -1
	for i := len(queued) - 1; i >= 0; i-- {
		job := queued[i]
		if job.isResolve() || job.announcesPage() {
			continue
		}
		if !job.urgent() {
			return i
		}
		if fallback < 0 {
			fallback = i
		}
	}
	return fallback
}

// announcesPage reports the first message of a page: if it is lost, the
// alert never opens.
func (j deliverJob) announcesPage() bool {
	return j.kind == jobIncident && j.incident != nil &&
		j.incident.IsPage() && j.incident.IsOpening()
}
