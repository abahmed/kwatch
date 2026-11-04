package dashboard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/webpush"
	"github.com/abahmed/kwatch/internal/storage"
)

// pushStateKey is where the push state lives in the state bucket.
const pushStateKey = "dashboard.push"

// Push bounds: the browsers kept, and how long one is kept after the
// dashboard was last open in it. Each time the dashboard opens with
// notifications on it subscribes again, which keeps it.
const (
	maxPushSubs = 100
	pushSubLife = 30 * 24 * time.Hour
)

// pushPrefs is what one browser wants told: issues at least as severe
// as Min (page, notify or digest), and resolves when Resolved.
type pushPrefs struct {
	Min      string `json:"min"`
	Resolved bool   `json:"resolved,omitempty"`
}

// pushSub is one browser that asked for notifications. Gen ties it to
// the sign-in secrets it subscribed under: a changed password or token
// forgets it.
type pushSub struct {
	Sub   webpush.Subscription `json:"sub"`
	Prefs pushPrefs            `json:"prefs"`
	Gen   string               `json:"gen"`
	Seen  time.Time            `json:"seen"`
}

// pushState is what is stored: the VAPID key and the browsers by ID.
type pushState struct {
	Key  string             `json:"key"`
	Subs map[string]pushSub `json:"subs,omitempty"`
}

// pushID names a browser by its push address, which only it knows.
func pushID(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:16])
}

// pushGen is the generation of the sign-in secrets.
func pushGen(password, token string) string {
	sum := sha256.Sum256([]byte("kwatch push\x00" + password + "\x00" +
		token))
	return hex.EncodeToString(sum[:8])
}

// pushBook holds the push state in memory and writes each change
// through to the store.
type pushBook struct {
	mu     sync.Mutex
	key    webpush.Key
	state  pushState
	values storage.Keyed[pushState]
	gen    string
	// failing is why the last send to a browser failed, by ID.
	failing map[string]string
}

// errPushStore says the push state could not be saved.
var errPushStore = errors.New("push state not saved")

// loadPushBook reads the stored state, makes the key on the first
// start, and forgets browsers of another sign-in generation or not
// seen for pushSubLife.
func loadPushBook(
	store *storage.Store, gen string, now time.Time,
) (*pushBook, error) {
	b := &pushBook{values: storage.StateValues[pushState](store),
		gen: gen, failing: map[string]string{}}
	state, _, err := b.values.Get(pushStateKey)
	if err != nil {
		return nil, err
	}
	key, err := webpush.ParseKey(state.Key)
	if state.Key == "" || err != nil {
		if key, err = webpush.NewKey(); err != nil {
			return nil, err
		}
		// A new key makes every old subscription useless.
		state = pushState{}
		if state.Key, err = key.Marshal(); err != nil {
			return nil, err
		}
	}
	b.key, b.state = key, state
	b.prune(now)
	if err := b.values.Put(pushStateKey, b.state); err != nil {
		return nil, err
	}
	return b, nil
}

// prune drops what loadPushBook forgets; the caller saves.
func (b *pushBook) prune(now time.Time) bool {
	changed := false
	for id, s := range b.state.Subs {
		if s.Gen != b.gen || now.Sub(s.Seen) > pushSubLife ||
			!s.Sub.Valid() {
			delete(b.state.Subs, id)
			changed = true
		}
	}
	return changed
}

// save writes the state; the caller holds mu.
func (b *pushBook) save() error {
	if err := b.values.Put(pushStateKey, b.state); err != nil {
		return errors.Join(errPushStore, err)
	}
	return nil
}

// subscribe adds or refreshes a browser, forgetting the address it
// replaces (empty: none). At the limit a browser makes room: one whose
// last notification failed first, then the one seen longest ago.
func (b *pushBook) subscribe(
	sub webpush.Subscription, prefs pushPrefs, replaces string,
	now time.Time,
) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := pushID(sub.Endpoint)
	if b.state.Subs == nil {
		b.state.Subs = map[string]pushSub{}
	}
	if old := pushID(replaces); replaces != "" && old != id {
		delete(b.state.Subs, old)
		delete(b.failing, old)
	}
	if _, ok := b.state.Subs[id]; !ok &&
		len(b.state.Subs) >= maxPushSubs {
		delete(b.state.Subs, b.oldest())
	}
	b.state.Subs[id] = pushSub{Sub: sub, Prefs: prefs, Gen: b.gen,
		Seen: now}
	delete(b.failing, id)
	return b.save()
}

// oldest is the ID of the browser to drop at the limit: failing ones
// first, then the one seen longest ago.
func (b *pushBook) oldest() string {
	ids := make([]string, 0, len(b.state.Subs))
	for id := range b.state.Subs {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(x, y string) int {
		_, fx := b.failing[x]
		_, fy := b.failing[y]
		if fx != fy {
			if fx {
				return -1
			}
			return 1
		}
		return b.state.Subs[x].Seen.Compare(b.state.Subs[y].Seen)
	})
	return ids[0]
}

// remove forgets a browser; unknown ones are fine.
func (b *pushBook) remove(endpoint string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := pushID(endpoint)
	if _, ok := b.state.Subs[id]; !ok {
		return nil
	}
	delete(b.state.Subs, id)
	delete(b.failing, id)
	return b.save()
}

// status says whether endpoint is subscribed and why sends to it fail.
func (b *pushBook) status(endpoint string) (bool, string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	id := pushID(endpoint)
	_, ok := b.state.Subs[id]
	return ok, b.failing[id]
}

// targets copies the browsers to send to, after forgetting the ones
// past pushSubLife.
func (b *pushBook) targets(now time.Time) map[string]pushSub {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.prune(now) {
		_ = b.save()
	}
	out := make(map[string]pushSub, len(b.state.Subs))
	for id, s := range b.state.Subs {
		out[id] = s
	}
	return out
}

// noteSend records how a send to id went: nil clears a failure, a
// gone browser is forgotten. It reports whether a failure is new, so
// it is logged once.
func (b *pushBook) noteSend(id string, err error) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil {
		delete(b.failing, id)
		return false
	}
	if webpush.Gone(err) {
		if _, ok := b.state.Subs[id]; ok {
			delete(b.state.Subs, id)
			_ = b.save()
		}
		delete(b.failing, id)
		return false
	}
	if _, ok := b.state.Subs[id]; !ok {
		return false
	}
	_, had := b.failing[id]
	b.failing[id] = pushFailure(err)
	return !had
}

// prefsOf returns the settings of the browser at endpoint.
func (b *pushBook) prefsOf(endpoint string) (pushPrefs, bool) {
	if endpoint == "" {
		return pushPrefs{}, false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.state.Subs[pushID(endpoint)]
	return s.Prefs, ok
}

// find returns the subscribed browser at endpoint.
func (b *pushBook) find(endpoint string) (webpush.Subscription, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.state.Subs[pushID(endpoint)]
	return s.Sub, ok
}
