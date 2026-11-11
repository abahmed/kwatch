package email

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// Mail clients thread on Message-ID, In-Reply-To and References, and
// some filters penalise a mail without a Message-ID. Every incident
// mail gets a Message-ID that is the same on every retry of that mail,
// and every mail after the opening names the opening's ID, so updates
// and the resolve are shown under it.
//
// Notices, summaries and digests get none: they are not conversations,
// and two of them could share a key and revision, which a mail server
// may take for one mail delivered twice.

// threadIDs returns the Message-ID of msg and the ID of the opening it
// answers; both are empty for a message that is not an incident, and
// parent is empty for the opening itself.
func threadIDs(
	msg notification.Message, cluster, domain string,
) (id, parent string) {
	if msg.Key == "" || msg.IsInformational() {
		return "", ""
	}
	base := shortHash(msg.AlertKey(cluster)+"|"+msg.ThreadKey(), 12)
	opening := fmt.Sprintf("<kwatch.%s.1@%s>", base, domain)
	if msg.Revision <= 1 && !msg.Resolved() {
		return opening, ""
	}
	// Two different mails must never share an ID, also when they share
	// a revision, so what the mail says is part of it.
	content := shortHash(
		fmt.Sprintf("%d|%s", msg.Status, msg.MailSubject()), 4)
	return fmt.Sprintf("<kwatch.%s.%d.%s@%s>",
		base, msg.Revision, content, domain), opening
}

func shortHash(text string, bytes int) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:bytes])
}

// idDomain is the right-hand side of the Message-IDs: the domain of
// the from address.
func idDomain(from string) string {
	_, domain, found := strings.Cut(from, "@")
	domain = strings.Trim(strings.TrimSpace(domain), ".")
	if !found || domain == "" || strings.ContainsAny(domain, " <>@\r\n") {
		return "kwatch.invalid"
	}
	return strings.ToLower(domain)
}
