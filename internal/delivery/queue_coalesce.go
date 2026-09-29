package delivery

// offerQueuedJob coalesces a full provider queue without growing it. The
// manager lock serializes producers; the worker may still consume jobs.
func offerQueuedJob(
	queue chan deliverJob,
	job deliverJob,
) bool {
	select {
	case queue <- job:
		return true
	default:
	}
	queued := drainQueuedJobs(queue)
	index := replacementIndex(queued, job)
	if index >= 0 {
		// The newer message of the same conversation supersedes the
		// queued one in place: it carries everything the older one said
		// and keeps the conversation's position in the queue.
		queued[index] = job
		refillQueuedJobs(queue, queued)
		return true
	}
	if len(queued) < cap(queue) {
		queued = append(queued, job)
		refillQueuedJobs(queue, queued)
		return true
	}
	refillQueuedJobs(queue, queued)
	return false
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

// replacementIndex finds a queued story of the same conversation.
func replacementIndex(queued []deliverJob, arriving deliverJob) int {
	if arriving.kind != jobStory || arriving.story == nil {
		return -1
	}
	for i := len(queued) - 1; i >= 0; i-- {
		old := queued[i]
		if old.kind == jobStory && old.story != nil &&
			old.story.Key == arriving.story.Key {
			return i
		}
	}
	return -1
}
