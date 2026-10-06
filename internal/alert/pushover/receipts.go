package pushover

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// maxReceipts bounds the remembered receipts. Pushover expires an
// emergency by itself (at most three hours), so an old receipt is
// worthless; the bound only stops a long-running process from growing.
const maxReceipts = 1000

// rememberReceipt stores the receipt of an emergency message, if the
// answer carries one.
func (p *Pushover) rememberReceipt(key string, body []byte) {
	var answer struct {
		Receipt string `json:"receipt"`
	}
	if json.Unmarshal(body, &answer) != nil || answer.Receipt == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.receipts) >= maxReceipts {
		p.receipts = map[string]string{}
	}
	p.receipts[key] = answer.Receipt
}

// cancelReceipt stops the emergency alarm started for key. Nothing is
// remembered for most incidents (only page-tier ones start an alarm),
// which is not an error. The receipt is forgotten once Pushover answers,
// so a retry of a failed cancel tries again.
func (p *Pushover) cancelReceipt(ctx context.Context, key string) error {
	p.mu.Lock()
	receipt := p.receipts[key]
	p.mu.Unlock()
	if receipt == "" {
		return nil
	}
	form := url.Values{}
	form.Set("token", p.token)
	_, err := p.sender.Send(ctx, transport.Request{
		Provider: p.Name(), URL: p.cancelURL(receipt),
		Body:        []byte(form.Encode()),
		ContentType: "application/x-www-form-urlencoded",
	})
	if err != nil && !transport.IsPermanent(err) {
		return err
	}
	// Success, or a permanent refusal such as an expired receipt.
	p.mu.Lock()
	delete(p.receipts, key)
	p.mu.Unlock()
	return nil
}

// cancelURL is the receipts endpoint next to the messages endpoint.
func (p *Pushover) cancelURL(receipt string) string {
	base := p.url
	if i := strings.LastIndex(base, "/messages.json"); i >= 0 {
		base = base[:i]
	} else {
		base = strings.TrimRight(base, "/")
	}
	return base + "/receipts/" + url.PathEscape(receipt) + "/cancel.json"
}

// SnapshotThreads implements delivery.ThreadStateProvider so a restart
// between the announcement and the resolve still cancels the alarm.
func (p *Pushover) SnapshotThreads() map[string]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.receipts) == 0 {
		return nil
	}
	out := make(map[string]string, len(p.receipts))
	for key, receipt := range p.receipts {
		out[key] = receipt
	}
	return out
}

// RestoreThreads implements delivery.ThreadStateProvider.
func (p *Pushover) RestoreThreads(saved map[string]string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for key, receipt := range saved {
		if key != "" && receipt != "" && len(p.receipts) < maxReceipts {
			p.receipts[key] = receipt
		}
	}
}
