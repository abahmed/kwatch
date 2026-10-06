package detectors

import "github.com/abahmed/kwatch/internal/inventory"

// eligibleNodes names the nodes that pass every check of the pod except
// resources. It is nil, meaning every node, when none does: then the
// pod's rules exclude all of them and no subset is more telling.
func eligibleNodes(
	model inventory.Reader, pod inventory.EntityID,
) map[string]bool {
	entity, ok := model.Entity(pod)
	if !ok {
		return nil
	}
	c := newFitCase(model, entity)
	eligible := map[string]bool{}
	for _, n := range c.nodes {
		if onlyResourceBlockers(c.check(n)) {
			eligible[n.id.Name] = true
		}
	}
	if len(eligible) == 0 {
		return nil
	}
	return eligible
}

func onlyResourceBlockers(list []blocker) bool {
	for _, b := range list {
		if b.kind != blockResource {
			return false
		}
	}
	return true
}
