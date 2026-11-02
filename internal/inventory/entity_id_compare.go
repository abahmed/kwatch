package inventory

import (
	"cmp"
	"strings"
)

// CompareIDs orders a and b exactly as strings.Compare(a.String(),
// b.String()) does, without building either key: sorting thousands of
// identifiers by their String allocated two keys per comparison.
func CompareIDs(a, b EntityID) int {
	if a.Kind == b.Kind && a.Group == b.Group &&
		a.Namespace == b.Namespace {
		return strings.Compare(a.Name, b.Name)
	}
	ca, cb := keyCursor{parts: keyParts(a)}, keyCursor{parts: keyParts(b)}
	for {
		x, moreA := ca.next()
		y, moreB := cb.next()
		if !moreA || !moreB {
			return cmp.Compare(boolInt(moreA), boolInt(moreB))
		}
		if x != y {
			return cmp.Compare(x, y)
		}
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// keyCursor walks the bytes of a key given as the pieces String joins.
type keyCursor struct {
	parts [7]string
	i, x  int
}

// next returns the next byte of the key, false at its end.
func (c *keyCursor) next() (byte, bool) {
	for c.i < len(c.parts) && c.x == len(c.parts[c.i]) {
		c.i, c.x = c.i+1, 0
	}
	if c.i == len(c.parts) {
		return 0, false
	}
	b := c.parts[c.i][c.x]
	c.x++
	return b, true
}

// keyParts are the pieces String joins, in order; empty pieces add
// nothing.
func keyParts(id EntityID) [7]string {
	dot := ""
	if id.Group != "" {
		dot = "."
	}
	return [7]string{
		string(id.Kind), dot, id.Group, "/", id.Namespace, "/", id.Name,
	}
}
