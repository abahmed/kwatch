package replay

import (
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/redact"
)

// Scrubbed free text. With SanitizeOptions.ScrubText, messages, log
// lines and error text are kept instead of dropped, because they are
// what most causes are read from (a crash's first error line, a Warning
// event, a condition message). They are scrubbed first:
//
//   - credentials go, as everywhere kwatch keeps text (redact.Credentials);
//   - a name the log already hid (an object or namespace name) gets the
//     same pseudonym inside the text, whatever its case, so a message
//     still points at the object it names ("dial tcp name-3f..:9000" is
//     that Service); a name after a Kubernetes kind is hidden even when
//     the log never saw the object (sanitize_names.go);
//   - container images, email addresses and outside host names get
//     pseudonyms of their own;
//   - the rest (words, codes, numbers, addresses, ports) is kept.
//
// A text is cut to maxScrubbedText bytes.

// maxScrubbedText bounds one kept text, in bytes.
const maxScrubbedText = 512

// maxKnownNames bounds the names remembered for scrubbing; past it, new
// names are still hidden as objects but no longer inside text.
const maxKnownNames = 20000

// minKnownName is the shortest name hidden inside text: shorter ones
// ("db", "ui") are too often plain words.
const minKnownName = 3

// knownNames maps names the log hid to their pseudonyms.
type knownNames struct {
	mu    sync.Mutex
	names map[string]string
}

func (k *knownNames) remember(name, pseudonym string) {
	if len(name) < minKnownName {
		return
	}
	name = strings.ToLower(name)
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.names == nil {
		k.names = map[string]string{}
	}
	if _, ok := k.names[name]; ok || len(k.names) >= maxKnownNames {
		return
	}
	k.names[name] = pseudonym
}

func (k *knownNames) lookup(name string) (string, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	p, ok := k.names[name]
	return p, ok
}

var (
	// imageRef is a container image named in text: a registry or
	// repository path with a tag or digest.
	imageRef = regexp.MustCompile(`\b[a-z0-9][a-z0-9.-]*(?::[0-9]+)?` +
		`(?:/[a-z0-9._-]+)+(?::[A-Za-z0-9._-]+|@sha256:[a-f0-9]{16,})`)
	emailAddress = regexp.MustCompile(
		`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+\b`)
)

// systemDomains are Kubernetes' own domains, kept as they are: they name
// taints, labels and APIs ("node.kubernetes.io/not-ready"), not anyone's
// systems.
var systemDomains = []string{"kubernetes.io", "k8s.io", "x-k8s.io"}

// maxScrubCache bounds the cached first stage of scrubbing; when full it
// starts over.
const maxScrubCache = 4096

// scrubCache keeps the first, name-independent stage of scrubbing
// (credentials, images, emails) per text: the same status message
// repeats across many pods, and credential redaction is the costly part.
type scrubCache struct {
	mu   sync.Mutex
	done map[string]string
}

func (c *scrubCache) get(text string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	out, ok := c.done[text]
	return out, ok
}

func (c *scrubCache) put(text, out string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done == nil || len(c.done) >= maxScrubCache {
		c.done = map[string]string{}
	}
	c.done[text] = out
}

// scrubText is text kept, scrubbed and bounded.
func (s *Sanitizer) scrubText(text string) string {
	first, ok := s.firstStage.get(text)
	if !ok {
		first = redact.Credentials(text)
		first = imageRef.ReplaceAllStringFunc(first, s.image)
		first = emailAddress.ReplaceAllStringFunc(first,
			func(m string) string { return s.pseudonym("email", m) })
		s.firstStage.put(text, first)
	}
	// Names are learned as the log goes, so this pass always runs.
	return cut(s.scrubNames(first), maxScrubbedText)
}

// cut keeps at most n bytes of text, on a rune boundary, marked "…".
func cut(text string, n int) string {
	if len(text) <= n {
		return text
	}
	end := n
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end] + "…"
}
