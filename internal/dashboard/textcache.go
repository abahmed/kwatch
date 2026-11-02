package dashboard

import (
	"sync"

	"github.com/abahmed/kwatch/internal/redact"
)

// maxCachedTexts bounds each generation of the redaction cache, so it
// holds at most twice this many texts.
const maxCachedTexts = 8192

// textCache remembers redacted titles and causes between list reads:
// with thousands of incidents a poll would otherwise redact every one
// again, and most texts repeat (one error in many places). It keeps two
// generations: when the current one is full it becomes the previous
// one and a new one starts. A text read from the previous generation
// moves to the current one, so texts still in use survive the switch
// and only the ones nobody read for a whole generation are dropped.
type textCache struct {
	mu   sync.Mutex
	cur  map[string]string
	prev map[string]string
	// fn redacts a text the cache has not seen; nil is
	// redact.Evidence.
	fn func(string) string
}

// redact is redact.Evidence of s, from the cache when it was seen.
func (c *textCache) redact(s string) string {
	if s == "" {
		return ""
	}
	c.mu.Lock()
	out, ok := c.cur[s]
	if !ok {
		if out, ok = c.prev[s]; ok {
			c.store(s, out)
		}
	}
	fn := c.fn
	c.mu.Unlock()
	if ok {
		return out
	}
	if fn == nil {
		fn = redact.Evidence
	}
	out = fn(s)
	c.mu.Lock()
	c.store(s, out)
	c.mu.Unlock()
	return out
}

// store puts a text in the current generation, starting a new one
// when it is full. The caller holds mu.
func (c *textCache) store(s, out string) {
	if c.cur == nil || len(c.cur) >= maxCachedTexts {
		c.prev = c.cur
		c.cur = make(map[string]string)
	}
	c.cur[s] = out
}
