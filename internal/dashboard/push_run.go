package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/delivery/webpush"
	"github.com/abahmed/kwatch/internal/storage"
)

// Push timing: how often the issues are compared, how long one send
// may take, and how long a push service keeps a message for a browser
// that is offline (an old alert is worth little).
const (
	pushEvery   = 10 * time.Second
	pushTimeout = 10 * time.Second
	pushTTL     = time.Hour
)

// pusher sends Web Push notifications to the browsers that asked.
type pusher struct {
	book *pushBook
	send transport.Sender
	prev pushSnapshot
}

// RunPush sends browser notifications until ctx ends: every pushEvery
// it compares the issues with the last check and pushes what changed
// to each subscribed browser. It runs with the pipeline, whose store
// keeps the key and the browsers; /api/push answers 503 without it.
// A state that can't be read yet is tried again at every tick.
func (s *Server) RunPush(
	ctx context.Context, store *storage.Store, send transport.Sender,
) {
	defer s.sources.push.Store(nil)
	var p *pusher
	logged := false
	tick := time.NewTicker(pushEvery)
	defer tick.Stop()
	for {
		// Without sign-in secrets nobody can ask for notifications, and
		// loading the book under the empty generation would drop the
		// browsers kept under the real one. The secrets may arrive later
		// (SetCredentials), so this is checked at every tick.
		if secret := s.auth.secret(); p == nil && secret.configured() {
			gen := pushGen(secret.rawPassword, secret.token)
			book, err := loadPushBook(store, gen, s.now.Now())
			if err == nil {
				p = &pusher{book: book, send: send}
				s.sources.push.Store(p)
			} else if !logged {
				logged = true
				klog.ErrorS(err, "dashboard: browser notifications "+
					"wait for the state file; trying again",
					"component", "dashboard-push")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if p != nil {
				p.check(ctx, s.pushItems, s.now.Now())
			}
		}
	}
}

// pushItems lists the issues as the dashboard shows them, or nil when
// the pipeline has none to give.
func (s *Server) pushItems() []IncidentListItem {
	reader := s.sources.incidents.Load()
	if reader == nil {
		return nil
	}
	all := (*reader).Incidents()
	out := make([]IncidentListItem, 0, len(all))
	for _, p := range all {
		out = append(out, toListItemWith(p, s.texts.redact))
	}
	return out
}

// check compares the issues (list reads them) with the last check
// and sends each browser what it asked for. The first check only
// remembers: nothing already there when kwatch started is news. With
// no browser subscribed it reads nothing, and the next subscriber
// starts from what is there then.
func (p *pusher) check(
	ctx context.Context, list func() []IncidentListItem, now time.Time,
) {
	targets := p.book.targets(now)
	if len(targets) == 0 {
		p.prev = nil
		return
	}
	items := list()
	if items == nil {
		return
	}
	next := newPushSnapshot(items)
	prev := p.prev
	p.prev = next
	if prev == nil {
		return
	}
	// A push service kwatch can't reach this check is not tried again
	// for its other browsers: a cluster without internet access would
	// wait out every timeout.
	down := map[string]error{}
	urgent := next.urgent()
	for id, sub := range targets {
		service := transport.LogURL(sub.Sub.Endpoint)
		for _, m := range pushMessages(pushChanges(prev, next,
			sub.Prefs)) {
			if ctx.Err() != nil {
				return
			}
			m.Badge = &urgent
			err, skip := down[service]
			if !skip {
				err = p.deliver(ctx, sub.Sub, m, now)
			}
			if err == nil {
				// The ID is the subscription's hash, never the
				// push address (a capability URL).
				klog.InfoS("dashboard: browser notification sent",
					"component", "dashboard-push", "browser", id[:8],
					"service", service, "issue", m.Tag)
			}
			if unreachable(err) {
				down[service] = err
			}
			if p.book.noteSend(id, err) {
				klog.InfoS("dashboard: browser notification not sent",
					"component", "dashboard-push", "browser", id[:8],
					"service", service, "error", err)
			}
			if err != nil {
				break
			}
		}
	}
}

// deliver sends one notification to one browser.
func (p *pusher) deliver(
	ctx context.Context, sub webpush.Subscription, m pushMessage,
	now time.Time,
) error {
	payload, err := json.Marshal(m)
	if err != nil {
		return err
	}
	req, err := webpush.Request(sub, payload, p.book.key, now, pushTTL)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, pushTimeout)
	defer cancel()
	_, err = p.send.Send(ctx, req)
	return err
}

// test sends one notification to sub now, for the "Send a test"
// button.
func (p *pusher) test(
	ctx context.Context, sub webpush.Subscription, now time.Time,
) error {
	err := p.deliver(ctx, sub, pushMessage{
		Title: "kwatch notifications work",
		Body:  "You'll get one like this when an issue needs you.",
		Tag:   "kwatch-test", URL: "/",
	}, now)
	p.book.noteSend(pushID(sub.Endpoint), err)
	return err
}

// unreachable reports whether err is a connection that failed: no
// answer at all, as opposed to an answer or a local error.
func unreachable(err error) bool {
	var u *url.Error
	return errors.As(err, &u)
}

// pushFailure says in plain words why a send failed, for the
// dashboard.
func pushFailure(err error) string {
	code, ok := transport.StatusOf(err)
	switch {
	case unreachable(err):
		return "kwatch can't reach your browser's push service. " +
			"The cluster may block outbound internet access."
	case !ok:
		return "kwatch couldn't send to your browser's push service. " +
			"Turn notifications off and on again."
	case code == 401 || code == 403:
		return "Your browser's push service refused kwatch's " +
			"signature. Turn notifications off and on again."
	case code == 429:
		return "Your browser's push service is limiting kwatch. " +
			"It will try again with the next change."
	}
	return "Your browser's push service answered with an error."
}
