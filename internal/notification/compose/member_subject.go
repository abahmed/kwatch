package compose

import (
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// aboutLead reports whether "it" and "its" in a sentence about the
// member id read as the lead's own subject: the member is the subject,
// or a pod or container that runs under it. For any other member (a
// container beside a node or Secret lead) the sentence must name it.
func (f caseFacts) aboutLead(id inventory.EntityID) bool {
	subject := leadSubject(f)
	if id == subject {
		return true
	}
	if id.Kind == kube.KindContainer {
		pod, _ := splitContainer(id.Name)
		id = inventory.CoreID(kube.KindPod, id.Namespace, pod)
		if id == subject {
			return true
		}
	}
	owner, ok := ownerIn([]inventory.EntityID{subject}, id)
	return ok && owner == subject
}

// memberName names the member a sentence is about, relative to the
// lead's root: "container app in pod api-7d9f in shop".
func (f caseFacts) memberName(id inventory.EntityID) string {
	return nameFrom(f.p.Root, id)
}
