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
// dropped: in a full queue with nothing to replace it takes the place of
// the newest queued job that is not a resolve, which goes to the
// overflow summary instead.
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
	refillQueuedJobs(queue, queued)
	return result
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
	index := newestNonResolve(queued)
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

// newestNonResolve finds the newest queued job that is not a resolve.
func newestNonResolve(queued []deliverJob) int {
	for i := len(queued) - 1; i >= 0; i-- {
		if !queued[i].isResolve() {
			return i
		}
	}
	return -1
}
