package detection

import (
	"reflect"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// sameModel reports whether a and b are the same reader. A reader that
// cannot be compared is never the same, so its pass is not shared.
func sameModel(a, b inventory.Reader) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta := reflect.TypeOf(a)
	return ta == reflect.TypeOf(b) && ta.Comparable() && a == b
}

// passCache keeps what one evaluation pass computes for many entities,
// such as the node list every unschedulable pod needs. A pass is the run
// of evaluations that share one Now: the engine reads the model once per
// pass and evaluates every changed entity at that instant, so the model
// does not change under it. A later Now starts a new pass and drops it,
// and so does a different model: a second evaluation at the same instant
// against another model must not reuse the first one's values.
type passCache struct {
	mu     sync.Mutex
	now    time.Time
	model  inventory.Reader
	values map[string]any
}

func (p *passCache) get(
	model inventory.Reader, now time.Time, key string, build func() any,
) any {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.values == nil || !p.now.Equal(now) || !sameModel(p.model, model) {
		p.now, p.model, p.values = now, model, map[string]any{}
	}
	if v, ok := p.values[key]; ok {
		return v
	}
	v := build()
	p.values[key] = v
	return v
}

// Pass returns the value build makes for key, made once per evaluation
// pass. Use it only for what is the same for every entity of the pass.
// Without a registry, as in detector unit tests, build runs every time.
func (c Context) Pass(key string, build func() any) any {
	if c.pass == nil {
		return build()
	}
	return c.pass.get(c.Model, c.Now, key, build)
}
