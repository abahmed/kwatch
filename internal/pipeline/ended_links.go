package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// endedLinks records the end of configured links (kube.LinkJournal)
// around every observation the engine applies, so a dependency removed
// just before a failure is a change the rules and messages can name.
// The zero value is ready; only the engine goroutine uses it.
type endedLinks struct {
	journal *kube.LinkJournal
}

// apply folds o into model, noting the configured links it ends.
func (l *endedLinks) apply(
	model *inventory.Model, o inventory.Observation,
) (inventory.Update, error) {
	if l.journal == nil {
		l.journal = kube.NewLinkJournal()
	}
	mark := l.journal.Before(model, o)
	update, err := model.Apply(o)
	if err == nil {
		l.journal.After(model, o, mark)
	}
	return update, err
}

// due returns, as observations to apply, the changes of the links that
// stayed gone for kube.LinkFlapWindow by now.
func (l *endedLinks) due(
	model *inventory.Model, now time.Time,
) []inventory.Observation {
	if l.journal == nil {
		return nil
	}
	return l.journal.Due(model, now)
}
