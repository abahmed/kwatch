package explain

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// scoreImageElsewhere asks whether the failing image runs healthy in
// another workload. If it does, the image itself works: a cause that
// blames the image loses weight and a cause that blames what surrounds
// it (config, environment, a dependency) gains it.
func scoreImageElsewhere(v *view, c *candidate) outcome {
	weight := 0.0
	switch v.blamesOf(c) {
	case blamesImage:
		weight = -ImageElsewhereWeight
	case blamesSurroundings:
		weight = ImageElsewhereWeight
	default:
		return outcome{}
	}
	units := v.failingUnits(c)
	failing := v.workloadsOfUnits(units)
	var image, twin inventory.EntityID
	for _, u := range units {
		for _, pod := range u.pods {
			if i, w, ok := v.imageTwin(pod, failing); ok {
				image, twin = i, w
				v.compare(u.effect, Comparison{
					Code: rootcause.ProofImageElsewhere, With: twin,
					Text: imageTwinText(image, twin)})
			}
		}
	}
	if twin == (inventory.EntityID{}) {
		return outcome{}
	}
	text := imageTwinText(image, twin)
	if weight > 0 {
		text = fmt.Sprintf("same image %s runs fine in %s, so the "+
			"image is not the difference", image.Name, shortName(twin))
	}
	return outcome{weight: weight, code: rootcause.ProofImageElsewhere,
		text: text}
}

// imageTwinText says that image runs fine in the twin workload.
func imageTwinText(image, twin inventory.EntityID) string {
	return fmt.Sprintf("same image %s runs fine in %s, so it is not "+
		"the image itself", image.Name, shortName(twin))
}

// imageTwin finds a healthy workload, other than the failing ones,
// that runs one of the images the failing pod's failing containers
// pull. It returns the image and that workload.
func (v *view) imageTwin(
	pod inventory.EntityID, failing map[inventory.EntityID]bool,
) (used, twin inventory.EntityID, ok bool) {
	for _, image := range v.failingImages(pod) {
		users := v.s.Model.Related(image, inventory.Pulls, inventory.Incoming)
		for _, user := range sampleIDs(users) {
			other, found := v.podOf(user)
			if !found || failing[v.workloadOf(other)] ||
				!v.healthyWorkload(other) {
				continue
			}
			return image, v.workloadOf(other), true
		}
	}
	return inventory.EntityID{}, inventory.EntityID{}, false
}

// failingImages are the images the pod's failing containers pull; when
// no container is singled out, every container's image but an init
// container's: a shared helper image proves nothing about the app.
func (v *view) failingImages(pod inventory.EntityID) []inventory.EntityID {
	var failing, all []inventory.EntityID
	for _, container := range v.s.Model.Related(
		pod, inventory.PartOf, inventory.Incoming,
	) {
		if e, ok := v.s.Model.Entity(container); ok {
			if init, _ := attributeValue(e, kube.AttrInit).AsBool(); init {
				continue
			}
		}
		images := v.s.Model.Related(container, inventory.Pulls,
			inventory.Outgoing)
		all = append(all, images...)
		if v.failing(container) {
			failing = append(failing, images...)
		}
	}
	if len(failing) > 0 {
		return failing
	}
	return all
}
