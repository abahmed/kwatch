package delivery

import "github.com/abahmed/kwatch/internal/model"

// offerQueuedJob coalesces a full provider queue without growing it. The
// manager lock serializes producers; the worker may still consume jobs.
func offerQueuedJob(
	queue chan deliverJob,
	job deliverJob,
) (bool, *deliverJob) {
	select {
	case queue <- job:
		return true, nil
	default:
	}
	queued := drainQueuedJobs(queue)
	index := replacementIndex(queued, job)
	if index >= 0 {
		previous := queued[index]
		if previous.kind == jobIncident &&
			previous.inc != nil && job.inc != nil &&
			previous.inc.Key == job.inc.Key {
			if previous.action == model.ActionCreate &&
				job.action == model.ActionUpdate {
				job.action = model.ActionCreate
			}
			queued[index] = job
			refillQueuedJobs(queue, queued)
			return true, nil
		}
		queued[index] = job
		refillQueuedJobs(queue, queued)
		return true, &previous
	}
	if len(queued) < cap(queue) {
		queued = append(queued, job)
		refillQueuedJobs(queue, queued)
		return true, nil
	}
	refillQueuedJobs(queue, queued)
	return false, nil
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

func replacementIndex(queued []deliverJob, arriving deliverJob) int {
	if arriving.kind == jobIncident && arriving.inc != nil {
		for i := len(queued) - 1; i >= 0; i-- {
			old := queued[i]
			if old.kind != jobIncident || old.inc == nil ||
				old.inc.Key != arriving.inc.Key {
				continue
			}
			if old.action == model.ActionUpdate ||
				(old.action == model.ActionCreate &&
					arriving.action == model.ActionUpdate) {
				return i
			}
		}
		if arriving.action == model.ActionCreate ||
			arriving.action == model.ActionResolved {
			for i := len(queued) - 1; i >= 0; i-- {
				if queued[i].kind == jobIncident &&
					queued[i].action == model.ActionUpdate {
					return i
				}
			}
		}
	}
	return -1
}
