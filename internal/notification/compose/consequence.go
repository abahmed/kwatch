package compose

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxNamed bounds how many affected names one sentence lists.
const maxNamed = 3

// week is the window recurrences are counted in.
const week = 7 * 24 * time.Hour

// impactSentences say who else is affected: "service cart can't serve
// traffic." and "checkout is affected as well."
func impactSentences(f caseFacts) []sentence {
	subject := leadSubject(f)
	var traffic, workloads []inventory.EntityID
	for _, id := range f.p.Impact {
		switch {
		case id == subject || id == f.p.Root:
		case f.ok && strings.Contains(f.lead.Summary, " "+id.Name+" "):
			// The lead already names it ("no ready pods behind
			// service policy-webhook").
		case id.Kind == kube.KindService || id.Kind == kube.KindIngress:
			traffic = append(traffic, id)
		case isWorkload(id.Kind):
			workloads = append(workloads, id)
		}
	}
	var out []sentence
	if len(traffic) > 0 {
		out = append(out, sentence{part: partConsequence, weight: 1,
			text: nameList(subject, traffic) + " can't serve traffic."})
	}
	if len(workloads) > 0 {
		out = append(out, sentence{part: partConsequence,
			text: nameList(subject, workloads) + " " +
				verb(len(workloads), "is", "are") + " affected as well."})
	}
	return out
}

// nameList names entities relative to home. When they all live in one
// other namespace it is said once ("api and web in shop"), and so is a
// kind they share ("pods a and b").
func nameList(home inventory.EntityID, ids []inventory.EntityID) string {
	namespace, shared := sharedNamespace(ids)
	if !shared {
		// "pod a in billing and pods b and c in shop"
		groups := byNamespace(limitIDs(ids, maxNamed))
		names := make([]string, 0, len(groups))
		for _, group := range groups {
			names = append(names, nameList(home, group))
		}
		if rest := len(ids) - maxNamed; rest > 0 {
			names = append(names, plural(rest, "other"))
		}
		return joinAnd(names, len(names))
	}
	place := ""
	if namespace != "" && namespace != home.Namespace {
		place = " in " + namespace
	}
	names := make([]string, 0, len(ids))
	kind, sameKind := sharedKind(ids)
	for _, id := range ids {
		if sameKind && len(ids) > 1 {
			names = append(names, id.Name)
		} else {
			names = append(names, shortName(id))
		}
	}
	if sameKind && len(ids) > 1 {
		return pluralWord(kindWord(kind)) + " " + joinAnd(names, maxNamed) +
			place
	}
	return joinAnd(names, maxNamed) + place
}

// sharedKind reports the kind of ids when they are all the same plain
// kind, which a list can say once.
func sharedKind(ids []inventory.EntityID) (inventory.Kind, bool) {
	kind := ids[0].Kind
	if isWorkload(kind) || kind == kube.KindContainer {
		return "", false
	}
	for _, id := range ids {
		if id.Kind != kind {
			return "", false
		}
	}
	return kind, true
}

// byNamespace groups ids by namespace, in the order namespaces first
// appear.
func byNamespace(ids []inventory.EntityID) [][]inventory.EntityID {
	index := map[string]int{}
	var groups [][]inventory.EntityID
	for _, id := range ids {
		i, ok := index[id.Namespace]
		if !ok {
			i = len(groups)
			index[id.Namespace] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], id)
	}
	return groups
}

func limitIDs(ids []inventory.EntityID, n int) []inventory.EntityID {
	if len(ids) > n {
		return ids[:n]
	}
	return ids
}

// sharedNamespace reports the namespace of ids when they all share one.
func sharedNamespace(ids []inventory.EntityID) (string, bool) {
	for _, id := range ids {
		if id.Namespace != ids[0].Namespace {
			return "", false
		}
	}
	return ids[0].Namespace, true
}

func verb(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// unverifiedSentences name what kwatch could not check.
func unverifiedSentences(f caseFacts) []sentence {
	line := unverifiedLine(f.p.Unverified)
	if line == "" {
		return nil
	}
	return []sentence{{part: partUnverified, text: line}}
}

// recurrenceSentences say that this has happened before, or keeps
// coming back.
func recurrenceSentences(f caseFacts) []sentence {
	if cycles := len(f.p.Cycles); cycles > 1 {
		return []sentence{{part: partRecurrence, text: "It has recovered " +
			"and failed again " + plural(cycles, "time") + " recently."}}
	}
	// This message is one time; earlier times count only when people
	// heard about them.
	times := 1
	for _, o := range f.p.History {
		if o.Heard && f.now.Sub(o.Opened) <= week {
			times++
		}
	}
	if times < 2 {
		return nil
	}
	return []sentence{{part: partRecurrence,
		text: "This is the " + ordinalWord(times) + " time this week."}}
}

// pluralWord writes the plural of a kind word: "pods", "ingresses",
// "network policies".
func pluralWord(word string) string {
	switch {
	case strings.HasSuffix(word, "s"):
		return word + "es"
	case strings.HasSuffix(word, "y") && !strings.HasSuffix(word, "ay"):
		return word[:len(word)-1] + "ies"
	}
	return word + "s"
}
