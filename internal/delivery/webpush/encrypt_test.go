package webpush

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/sha256"
	"strings"
	"testing"
)

// The example of RFC 8291, Section 5 and Appendix A.
const (
	rfcPlain     = "V2hlbiBJIGdyb3cgdXAsIEkgd2FudCB0byBiZSBhIHdhdGVybWVsb24"
	rfcASPrivate = "yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"
	rfcUAPublic  = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcx" +
		"aOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"
	rfcUAPrivate = "q1dXpw3UpT5VOmu_cf_v6ih07Aems3njxI-JWgLcM94"
	rfcSalt      = "DGv6ra1nlYgDCS1FRnbzlw"
	rfcAuth      = "BTBZMqHH6r4Tts7J_aSIgg"
	rfcHeader    = "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z" +
		"9KsN6nGRTbVYI_c7VJSPQTBtkgcy27ml" +
		"mlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8"
	rfcCipher = "8pfeW0KbunFT06SuDKoJH9Ql87S1QUrd" +
		"irN6GcG7sFz1y1sqLgVi1VhjVkHsUoEs" +
		"bI_0LpXMuGvnzQ"
)

func mustDecode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := decode(s)
	if err != nil {
		t.Fatalf("decode %q: %v", s, err)
	}
	return b
}

func TestSealMatchesRFC8291Example(t *testing.T) {
	local, err := ecdh.P256().NewPrivateKey(mustDecode(t, rfcASPrivate))
	if err != nil {
		t.Fatal(err)
	}
	keys := Keys{P256dh: rfcUAPublic, Auth: rfcAuth}
	browser, auth, err := keys.parse()
	if err != nil {
		t.Fatal(err)
	}
	got, err := seal(browser, auth, local, mustDecode(t, rfcSalt),
		mustDecode(t, rfcPlain))
	if err != nil {
		t.Fatal(err)
	}
	want := append(mustDecode(t, rfcHeader), mustDecode(t, rfcCipher)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("seal =\n%s\nwant\n%s", encode(got), encode(want))
	}
}

// open decrypts body as the browser would (RFC 8291, receiver side).
func open(t *testing.T, ua *ecdh.PrivateKey, auth, body []byte) []byte {
	t.Helper()
	salt, idlen := body[:16], int(body[20])
	ours := body[21 : 21+idlen]
	sender, err := ecdh.P256().NewPublicKey(ours)
	if err != nil {
		t.Fatal(err)
	}
	shared, err := ua.ECDH(sender)
	if err != nil {
		t.Fatal(err)
	}
	info := append([]byte("WebPush: info\x00"), ua.PublicKey().Bytes()...)
	info = append(info, ours...)
	ikm, _ := hkdf.Key(sha256.New, shared, auth, string(info), 32)
	cek, _ := hkdf.Key(sha256.New, ikm, salt,
		"Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, salt,
		"Content-Encoding: nonce\x00", 12)
	block, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(block)
	plain, err := gcm.Open(nil, nonce, body[21+idlen:], nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if plain[len(plain)-1] != 2 {
		t.Fatalf("last record delimiter = %d", plain[len(plain)-1])
	}
	return plain[:len(plain)-1]
}

func TestEncryptOpensForTheBrowserOnly(t *testing.T) {
	ua, err := ecdh.P256().NewPrivateKey(mustDecode(t, rfcUAPrivate))
	if err != nil {
		t.Fatal(err)
	}
	keys := Keys{P256dh: rfcUAPublic, Auth: rfcAuth}
	msg := []byte(`{"title":"Urgent: api crash-loops"}`)
	a, err := Encrypt(keys, msg)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := Encrypt(keys, msg)
	if bytes.Equal(a, b) {
		t.Fatal("two messages share a key pair and salt")
	}
	if got := open(t, ua, mustDecode(t, rfcAuth), a); !bytes.Equal(got, msg) {
		t.Fatalf("opened %q", got)
	}
}

func TestEncryptRefusesBadInput(t *testing.T) {
	good := Keys{P256dh: rfcUAPublic, Auth: rfcAuth}
	for name, keys := range map[string]Keys{
		"no key":       {Auth: rfcAuth},
		"not a point":  {P256dh: encode(make([]byte, 65)), Auth: rfcAuth},
		"short secret": {P256dh: rfcUAPublic, Auth: "AAAA"},
		"not base64":   {P256dh: "!!", Auth: rfcAuth},
	} {
		if _, err := Encrypt(keys, []byte("x")); err == nil {
			t.Errorf("%s: no error", name)
		}
		if keys.Valid() {
			t.Errorf("%s: Valid", name)
		}
	}
	big := []byte(strings.Repeat("x", MaxPayload+1))
	if _, err := Encrypt(good, big); err == nil {
		t.Error("payload over MaxPayload taken")
	}
	body, err := Encrypt(good, []byte(strings.Repeat("x", MaxPayload)))
	if err != nil || len(body) != 4096 {
		t.Fatalf("largest payload: %d bytes, %v", len(body), err)
	}
	if _, err := Encrypt(Keys{P256dh: rfcUAPublic + "==",
		Auth: rfcAuth + "=="}, []byte("x")); err != nil {
		t.Errorf("padded base64url refused: %v", err)
	}
}
