package compose

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var numberWords = []string{"zero", "one", "two", "three", "four", "five",
	"six", "seven", "eight", "nine", "ten"}

// numberWord spells small counts out ("six") and writes larger ones as
// digits, the way people write them in prose.
func numberWord(n int) string {
	if n >= 0 && n < len(numberWords) {
		return numberWords[n]
	}
	return strconv.Itoa(n)
}

var ordinalWords = []string{"", "first", "second", "third", "fourth",
	"fifth", "sixth", "seventh", "eighth", "ninth", "tenth"}

func ordinalWord(n int) string {
	if n > 0 && n < len(ordinalWords) {
		return ordinalWords[n]
	}
	return strconv.Itoa(n) + "th"
}

// plural writes "one minute" or "seven minutes".
func plural(n int, unit string) string {
	if n == 1 {
		return "one " + unit
	}
	return numberWord(n) + " " + unit + "s"
}

// humanDuration rounds a duration the way a person would say it:
// "less than a minute", "seven minutes", "about 2 hours", "six days".
func humanDuration(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "less than a minute"
	case d < time.Hour:
		return roughly(d.Minutes(), "minute")
	case d < 24*time.Hour:
		return roughly(d.Hours(), "hour")
	default:
		return roughly(d.Hours()/24, "day")
	}
}

// roughly rounds value and says "about" when rounding changed it by
// more than a tenth.
func roughly(value float64, unit string) string {
	n := int(math.Round(value))
	text := plural(n, unit)
	if math.Abs(value-float64(n)) > value/10 {
		return "about " + text
	}
	return text
}

// goDuration matches durations written by format.Duration or
// time.Duration.String: "72h0m0s", "5m30s", "45s", "2h30m", and the
// bare "2m" or "72h" format.Duration writes when the smaller units are
// zero.
var goDuration = regexp.MustCompile(
	`(?:\d+h)?(?:\d+m)?\d+(?:\.\d+)?s|\d+h(?:\d+m)?|\d+m`)

// durationWords introduce a time: "for 2m", "in 72h", "about 2h". A
// bare single unit is rewritten only after one of them or before
// "ago", because a CPU limit of 500m or a name ending in -12h is
// never introduced that way.
var durationWords = map[string]bool{
	"for": true, "in": true, "after": true, "within": true, "every": true,
	"than": true, "since": true, "over": true, "about": true, "past": true,
	"last": true,
}

// humanizeText rewrites machine durations inside detector text, so
// "expires in 72h0m0s" reads "expires in three days". A match glued to
// a word, a number or a path ("x-12h0m0s", "1.5m30s", "cpu=1h5m") is
// left alone.
func humanizeText(text string) string {
	var b strings.Builder
	last := 0
	for _, span := range goDuration.FindAllStringIndex(text, -1) {
		start, end := span[0], span[1]
		d, err := time.ParseDuration(text[start:end])
		if err != nil || !standsAlone(text, start, end) ||
			(bareUnit(text[start:end]) && !timeContext(text, start, end)) {
			continue
		}
		b.WriteString(text[last:start])
		b.WriteString(humanDuration(d))
		last = end
	}
	b.WriteString(text[last:])
	return b.String()
}

// standsAlone reports whether text[start:end] is its own token: not
// preceded by a letter, a digit, "-", ".", "=" or "/", and not followed
// by a letter, a digit, "-" or a "." that continues a word.
func standsAlone(text string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsLetter(before) || unicode.IsDigit(before) ||
			strings.ContainsRune("-.=/", before) {
			return false
		}
	}
	after, size := utf8.DecodeRuneInString(text[end:])
	switch {
	case size == 0:
		return true
	case unicode.IsLetter(after) || unicode.IsDigit(after) || after == '-':
		return false
	case after == '.':
		next, _ := utf8.DecodeRuneInString(text[end+size:])
		return !unicode.IsLetter(next) && !unicode.IsDigit(next)
	}
	return true
}

// bareUnit reports a duration of one unit without seconds ("2m",
// "72h"), the shape a quantity or a name can share.
func bareUnit(token string) bool {
	return !strings.ContainsRune(token, 's') &&
		strings.Count(token, "h")+strings.Count(token, "m") == 1
}

// timeContext reports whether the word before text[start:end] is one
// of durationWords or the word after it is "ago".
func timeContext(text string, start, end int) bool {
	if strings.HasPrefix(text[end:], " ago") {
		return true
	}
	words := strings.Fields(text[:start])
	return len(words) > 0 && durationWords[strings.ToLower(words[len(words)-1])]
}

var binaryUnits = map[string]float64{
	"Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30, "Ti": 1 << 40,
}

// humanBytes rounds a Kubernetes quantity ("11534Mi") or a plain byte
// count to "about 11 GiB". Values it cannot read are returned as is.
func humanBytes(value string) string {
	number, unit := value, ""
	if len(value) > 2 {
		if _, ok := binaryUnits[value[len(value)-2:]]; ok {
			number, unit = value[:len(value)-2], value[len(value)-2:]
		}
	}
	n, err := strconv.ParseFloat(number, 64)
	if err != nil || (unit == "" && n < 1<<10) {
		return value
	}
	if unit != "" {
		n *= binaryUnits[unit]
	}
	for _, u := range []string{"TiB", "GiB", "MiB", "KiB"} {
		size := binaryUnits[u[:2]]
		if n >= size {
			return roughlyNumber(n/size) + " " + u
		}
	}
	return value
}

func roughlyNumber(value float64) string {
	rounded := math.Round(value)
	if math.Abs(value-rounded) > 0.05 {
		return fmt.Sprintf("about %d", int(rounded))
	}
	return fmt.Sprintf("%d", int(rounded))
}

// clock is the minute of t in UTC, as people read it in a chat ("14:02").
func clock(t time.Time) string { return t.UTC().Format("15:04") }

// joinAnd lists values as "a", "a and b", or "a, b and c", naming at
// most n and counting the rest.
func joinAnd(values []string, n int) string {
	shown := limit(values, n)
	if rest := len(values) - len(shown); rest > 0 {
		shown = append(append([]string(nil), shown...),
			plural(rest, "other"))
	}
	if len(shown) < 2 {
		return strings.Join(shown, "")
	}
	last := len(shown) - 1
	return strings.Join(shown[:last], ", ") + " and " + shown[last]
}

// clip shortens a quote to at most maxQuote runes on a word boundary.
func clip(text string) string {
	const n = maxQuote
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= n {
		return string(runes)
	}
	cut := string(runes[:n])
	if i := strings.LastIndex(cut, " "); i > n/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,;:.") + "…"
}

// sentenceCase capitalises text and ends it with a full stop.
func sentenceCase(text string) string {
	return upperFirst(endSentence(text))
}

// endSentence ends text with a full stop. It keeps the first letter as
// is, because notes often start with a resource name ("payments").
func endSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" || strings.HasSuffix(text, ".") ||
		strings.HasSuffix(text, "…") {
		return text
	}
	return text + "."
}
