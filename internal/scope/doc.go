// Package scope decides which findings are in the configured delivery
// scope: namespaces, reasons, the namespace selector, silence rules and
// maintenance holds. In: compiled runtime scope configuration and the
// inventory model. Out: an allow decision per finding.
package scope
