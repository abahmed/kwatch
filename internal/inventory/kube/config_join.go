package kube

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// ConfigSource is the observation source of the dependencies pods read
// through ConfigMaps. It is apart from ObservationSource so the edges it
// owns are replaced without touching the pod's own.
const ConfigSource = "kubernetes-config"

// maxJoinedPods bounds the pods one ConfigMap change re-describes.
const maxJoinedPods = 1000

// ConfigJoin gives a pod a Calls edge to every external endpoint named
// by a ConfigMap it reads through its environment. A pod and the
// ConfigMap reach the source in any order, so the join keeps both sides
// and re-describes the pods whenever either changes. It keeps host and
// port only; a ConfigMap's values never reach it.
type ConfigJoin struct {
	mu   sync.Mutex
	maps map[inventory.EntityID][]configTarget
	pods map[inventory.EntityID]podConfig
	// readers are the pods that read a ConfigMap.
	readers map[inventory.EntityID]map[inventory.EntityID]bool
	// told is what was last emitted per pod and per endpoint, so an
	// unchanged pod update adds no observation.
	told map[inventory.EntityID]string
	said map[string]string
}

// podConfig is how one pod reads ConfigMaps: whole, or single keys.
type podConfig struct {
	whole map[string]bool
	keys  map[string][]string
}

// NewConfigJoin returns an empty join.
func NewConfigJoin() *ConfigJoin {
	return &ConfigJoin{
		maps:    map[inventory.EntityID][]configTarget{},
		pods:    map[inventory.EntityID]podConfig{},
		readers: map[inventory.EntityID]map[inventory.EntityID]bool{},
		told:    map[inventory.EntityID]string{},
		said:    map[string]string{},
	}
}

// Observed takes a described pod or ConfigMap and returns the
// observations its arrival adds. Other kinds add none.
func (j *ConfigJoin) Observed(
	desc Description, at time.Time,
) []inventory.Observation {
	switch desc.ID.Kind {
	case KindPod:
		return j.observePod(desc, at)
	case KindConfigMap:
		return j.observeConfigMap(desc, at)
	}
	return nil
}

// Gone takes the removal of a pod or ConfigMap.
func (j *ConfigJoin) Gone(
	id inventory.EntityID, at time.Time,
) []inventory.Observation {
	switch id.Kind {
	case KindPod:
		return j.forgetPod(id, at)
	case KindConfigMap:
		return j.refresh(id, nil, at)
	}
	return nil
}

func (j *ConfigJoin) observePod(
	desc Description, at time.Time,
) []inventory.Observation {
	j.mu.Lock()
	defer j.mu.Unlock()
	before := j.endpointsOf(desc.ID)
	j.unlink(desc.ID)
	refs := desc.Attributes[AttrConfigEnvRefs]
	config := parsePodConfig(refs.AsText())
	if len(config.whole)+len(config.keys) == 0 {
		delete(j.pods, desc.ID)
	} else {
		j.pods[desc.ID] = config
		j.link(desc.ID, config)
	}
	return j.emit([]inventory.EntityID{desc.ID}, before, at)
}

func (j *ConfigJoin) observeConfigMap(
	desc Description, at time.Time,
) []inventory.Observation {
	text := desc.Attributes[AttrConfigTargets]
	return j.refresh(desc.ID, parseTargets(text.AsText()), at)
}

// refresh sets a ConfigMap's targets and re-describes its readers.
func (j *ConfigJoin) refresh(
	id inventory.EntityID, targets []configTarget, at time.Time,
) []inventory.Observation {
	j.mu.Lock()
	defer j.mu.Unlock()
	readers := j.readerList(id)
	before := map[inventory.EntityID][]string{}
	for _, pod := range readers {
		before[pod] = j.endpointsOf(pod)
	}
	if len(targets) == 0 {
		delete(j.maps, id)
	} else {
		j.maps[id] = targets
	}
	return j.emit(readers, joinBefore(before), at)
}

func (j *ConfigJoin) forgetPod(
	id inventory.EntityID, at time.Time,
) []inventory.Observation {
	j.mu.Lock()
	defer j.mu.Unlock()
	before := j.endpointsOf(id)
	j.unlink(id)
	delete(j.pods, id)
	delete(j.told, id)
	return j.emit(nil, before, at)
}

// joinBefore flattens the endpoints the pods had before a change.
func joinBefore(before map[inventory.EntityID][]string) []string {
	var out []string
	for _, endpoints := range before {
		out = append(out, endpoints...)
	}
	return out
}

func (j *ConfigJoin) readerList(cm inventory.EntityID) []inventory.EntityID {
	var out []inventory.EntityID
	for pod := range j.readers[cm] {
		out = append(out, pod)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	if len(out) > maxJoinedPods {
		out = out[:maxJoinedPods]
	}
	return out
}

func (j *ConfigJoin) link(pod inventory.EntityID, config podConfig) {
	for _, name := range config.names() {
		cm := inventory.CoreID(KindConfigMap, pod.Namespace, name)
		if j.readers[cm] == nil {
			j.readers[cm] = map[inventory.EntityID]bool{}
		}
		j.readers[cm][pod] = true
	}
}

func (j *ConfigJoin) unlink(pod inventory.EntityID) {
	old, ok := j.pods[pod]
	if !ok {
		return
	}
	for _, name := range old.names() {
		cm := inventory.CoreID(KindConfigMap, pod.Namespace, name)
		delete(j.readers[cm], pod)
		if len(j.readers[cm]) == 0 {
			delete(j.readers, cm)
		}
	}
}

// edges are the endpoints, with the key each came from, that a pod
// reads through its ConfigMaps.
func (j *ConfigJoin) edges(pod inventory.EntityID) []configEdge {
	config, ok := j.pods[pod]
	if !ok {
		return nil
	}
	var out []configEdge
	for _, name := range config.names() {
		cm := inventory.CoreID(KindConfigMap, pod.Namespace, name)
		for _, target := range j.maps[cm] {
			if config.reads(name, target.key) {
				out = append(out, configEdge{cm: cm, target: target})
			}
		}
	}
	return out
}

type configEdge struct {
	cm     inventory.EntityID
	target configTarget
}

func (j *ConfigJoin) endpointsOf(pod inventory.EntityID) []string {
	var out []string
	for _, edge := range j.edges(pod) {
		out = append(out, edge.target.endpoint)
	}
	return out
}

// emit writes the pods' edges, then the sources of every endpoint the
// change touched, before or after.
func (j *ConfigJoin) emit(
	pods []inventory.EntityID, before []string, at time.Time,
) []inventory.Observation {
	touched := map[string]bool{}
	for _, endpoint := range before {
		touched[endpoint] = true
	}
	var out []inventory.Observation
	for _, pod := range pods {
		var targets []inventory.EntityID
		seen := map[string]bool{}
		for _, edge := range j.edges(pod) {
			touched[edge.target.endpoint] = true
			if !seen[edge.target.endpoint] {
				seen[edge.target.endpoint] = true
				targets = append(targets, endpointOf(edge.target.endpoint))
			}
		}
		sort.Slice(targets, func(a, b int) bool {
			return targets[a].Name < targets[b].Name
		})
		if len(targets) > maxPodDependencies {
			targets = targets[:maxPodDependencies]
		}
		if !j.changed(pod, targets) {
			continue
		}
		out = append(out, inventory.Observation{
			Kind: inventory.Related, Source: ConfigSource, At: at,
			Entity: pod, Relation: inventory.Calls, Targets: targets,
		})
	}
	return append(out, j.sourceObservations(touched, at)...)
}

// sourceObservations says, for each touched endpoint, which ConfigMap
// keys the pods read its address from. An endpoint nobody reads through
// a ConfigMap any more gets an empty observation, which removes them.
func (j *ConfigJoin) sourceObservations(
	touched map[string]bool, at time.Time,
) []inventory.Observation {
	var names []string
	for endpoint := range touched {
		names = append(names, endpoint)
	}
	sort.Strings(names)
	var out []inventory.Observation
	for _, endpoint := range names {
		var attrs map[string]inventory.Value
		if sources := j.sourcesOf(endpoint); len(sources) > 0 {
			attrs = map[string]inventory.Value{AttrEndpointSources: inventory.
				Text(strings.Join(sources, ","))}
		}
		text := attrs[AttrEndpointSources].AsText()
		if j.said[endpoint] == text {
			continue
		}
		j.said[endpoint] = text
		out = append(out, inventory.Observation{
			Kind: inventory.Observed, Source: ConfigSource, At: at,
			Entity: endpointOf(endpoint), Attributes: attrs,
		})
	}
	return out
}

// sourcesOf lists "namespace/name#KEY" for the keys of the ConfigMaps
// that name the endpoint and that a pod reads.
func (j *ConfigJoin) sourcesOf(endpoint string) []string {
	var out []string
	for cm, targets := range j.maps {
		for _, target := range targets {
			if target.endpoint == endpoint && j.read(cm, target.key) {
				out = append(out, cm.Namespace+"/"+cm.Name+"#"+target.key)
			}
		}
	}
	sort.Strings(out)
	if len(out) > maxEndpointSources {
		out = out[:maxEndpointSources]
	}
	return out
}

// read reports whether a pod reads the key of the ConfigMap.
func (j *ConfigJoin) read(cm inventory.EntityID, key string) bool {
	for pod := range j.readers[cm] {
		if j.pods[pod].reads(cm.Name, key) {
			return true
		}
	}
	return false
}

func parsePodConfig(refs string) podConfig {
	config := podConfig{whole: map[string]bool{}, keys: map[string][]string{}}
	for _, ref := range strings.Split(refs, ",") {
		if ref == "" {
			continue
		}
		name, key, single := strings.Cut(ref, "/")
		if single {
			config.keys[name] = append(config.keys[name], key)
		} else {
			config.whole[name] = true
		}
	}
	return config
}

func (c podConfig) names() []string {
	seen := map[string]bool{}
	var out []string
	for name := range c.whole {
		seen[name] = true
		out = append(out, name)
	}
	for name := range c.keys {
		if !seen[name] {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func (c podConfig) reads(name, key string) bool {
	if c.whole[name] {
		return true
	}
	for _, k := range c.keys[name] {
		if k == key {
			return true
		}
	}
	return false
}

// changed records the targets told for a pod and reports whether they
// differ from the last ones.
func (j *ConfigJoin) changed(
	pod inventory.EntityID, targets []inventory.EntityID,
) bool {
	names := make([]string, len(targets))
	for i, target := range targets {
		names[i] = target.Name
	}
	text := strings.Join(names, ",")
	if j.told[pod] == text {
		return false
	}
	if text == "" {
		delete(j.told, pod)
	} else {
		j.told[pod] = text
	}
	return true
}
