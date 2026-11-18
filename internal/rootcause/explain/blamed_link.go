package explain

import "github.com/abahmed/kwatch/internal/inventory"

// latestEdit is the change a cause records of its blamed changes,
// oldest first: the latest edit, not the end of a link it brought about
// (inventory.Change.RemovedLink). A selector edit that stopped a Service
// routing to a workload is what someone sets back; the link that ended
// is what it did. With only ended links, the latest one.
func latestEdit(blamed []inventory.Change) inventory.Change {
	for i := len(blamed) - 1; i >= 0; i-- {
		if _, _, ended := blamed[i].RemovedLink(); !ended {
			return blamed[i]
		}
	}
	return blamed[len(blamed)-1]
}
