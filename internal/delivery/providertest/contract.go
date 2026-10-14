package providertest

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/notification"
)

// Contract describes one provider under the contract test.
type Contract struct {
	// Send delivers one message through the provider.
	Send func(context.Context, notification.Message) error
	// Rec is the recorder the provider talks to.
	Rec *Recorder
	// Headers are the request headers to pin besides Content-Type.
	Headers []string
	// Secrets are literal credentials to redact from the golden.
	Secrets []string
	// LimitBytes is the provider's size limit the near-limit case
	// approaches; zero uses 3000.
	LimitBytes int
	// KeyOf pulls the incident or dedup key out of a request. When set,
	// open, update and resolve must all carry the same key.
	KeyOf func(t *testing.T, r Request) string
	// Check inspects the full request of a case; it is the place for
	// API-shape assertions. It is called even when nothing was sent.
	Check func(t *testing.T, name string, reqs []Request)
	// Chat marks a chat provider: it also renders the mentions case, and
	// no request may carry a live broadcast mention.
	Chat bool
	// Dir holds the goldens; empty means testdata/contract.
	Dir string
}

// RunContract sends every canonical message through the provider, in
// lifecycle order on one provider instance, and compares the exact
// requests with the committed goldens.
func RunContract(t *testing.T, c Contract) {
	t.Helper()
	dir := c.Dir
	if dir == "" {
		dir = filepath.Join("testdata", "contract")
	}
	opts := SnapshotOptions{Headers: c.Headers, Secrets: c.Secrets}
	keys := map[string]string{}
	cases := Canonical(c.LimitBytes)
	if c.Chat {
		cases = append(cases, ContractCase{
			Name: "mentions", Message: Mentions()})
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			reqs := c.runCase(t, tc, dir, opts)
			if c.KeyOf != nil && len(reqs) > 0 && tc.Lifecycle {
				keys[tc.Name] = c.KeyOf(t, reqs[len(reqs)-1])
			}
		})
	}
	if c.KeyOf != nil {
		assertSameKey(t, keys)
	}
}

func (c Contract) runCase(
	t *testing.T, tc ContractCase, dir string, opts SnapshotOptions,
) []Request {
	t.Helper()
	c.Rec.Reset()
	if err := c.Send(context.Background(), tc.Message); err != nil {
		t.Fatalf("send: %v", err)
	}
	reqs := c.Rec.Requests()
	if c.Chat {
		AssertNoMassMention(t, reqs)
	}
	CompareGolden(t, filepath.Join(dir, tc.Name+".golden"),
		snapshotAll(reqs, opts))
	if c.Check != nil {
		c.Check(t, tc.Name, reqs)
	}
	return reqs
}

func assertSameKey(t *testing.T, keys map[string]string) {
	t.Helper()
	first := keys["page-open"]
	for _, name := range []string{"notify-update", "resolve"} {
		if first == "" || keys[name] != first {
			t.Fatalf("key of %s = %q, want %q as page-open", name,
				keys[name], first)
		}
	}
}

func snapshotAll(reqs []Request, o SnapshotOptions) string {
	if len(reqs) == 0 {
		return "(no request sent)\n"
	}
	var parts []string
	for i, r := range reqs {
		s := Snapshot(r, o)
		if len(reqs) > 1 {
			s = fmt.Sprintf("--- request %d of %d\n%s", i+1, len(reqs), s)
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, "\n")
}

// AssertFencesBalanced fails when text has an odd number of ``` fences,
// which means a size cut broke a code block.
func AssertFencesBalanced(t *testing.T, text string) {
	t.Helper()
	if strings.Count(text, "```")%2 != 0 {
		t.Fatalf("code fence is open at the end of: %.200q", text)
	}
}
