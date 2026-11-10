package vonage

import "strings"

// gsm7Chars is the GSM 03.38 basic character set plus its extension
// table: the only characters a default (type=text) SMS can carry.
const gsm7Chars = "@£$¥èéùìòÇ\nØø\rÅåΔ_" +
	"ΦΓΛΩΠΨΣΘΞÆæßÉ !\"#¤%&'()*+,-./" +
	"0123456789:;<=>?¡ABCDEFGHIJKLMNOPQRSTUVWXYZÄÖÑÜ§¿" +
	"abcdefghijklmnopqrstuvwxyzäöñüà" +
	"\f^{}\\[~]|€"

// needsUnicode reports whether text has a character outside GSM-7, such
// as the status emoji. Sent as type=text it would arrive as garbage.
func needsUnicode(text string) bool {
	for _, r := range text {
		if !strings.ContainsRune(gsm7Chars, r) {
			return true
		}
	}
	return false
}
