package webpush

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// recordSize is the aes128gcm record size. One record holds any
// payload Encrypt takes.
const recordSize = 4096

// MaxPayload is the largest plaintext Encrypt takes: push services
// accept a 4096-byte body, and the header (86 bytes), the padding
// delimiter and the AES-GCM tag (17) take the rest.
const MaxPayload = 4096 - 86 - 17

// Keys are a browser's subscription keys, base64url as the browser's
// PushSubscription gives them: its P-256 public key and its 16-byte
// authentication secret.
type Keys struct {
	P256dh string `json:"p256dh"`
	Auth   string `json:"auth"`
}

// errKeys says the browser's keys are not usable.
var errKeys = errors.New("webpush: bad subscription keys")

// decode reads base64url with or without padding.
func decode(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "="))
}

// encode writes base64url without padding.
func encode(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

// parse checks keys and returns the browser's public key and secret.
func (k Keys) parse() (*ecdh.PublicKey, []byte, error) {
	raw, err := decode(k.P256dh)
	if err != nil {
		return nil, nil, errKeys
	}
	pub, err := ecdh.P256().NewPublicKey(raw)
	if err != nil {
		return nil, nil, errKeys
	}
	auth, err := decode(k.Auth)
	if err != nil || len(auth) != 16 {
		return nil, nil, errKeys
	}
	return pub, auth, nil
}

// Valid reports whether keys can be encrypted for.
func (k Keys) Valid() bool {
	_, _, err := k.parse()
	return err == nil
}

// Encrypt encrypts plaintext for the browser that holds keys, with a
// fresh key pair and salt for this message (RFC 8291).
func Encrypt(keys Keys, plaintext []byte) ([]byte, error) {
	if len(plaintext) > MaxPayload {
		return nil, fmt.Errorf("webpush: payload of %d bytes is over "+
			"the %d byte limit", len(plaintext), MaxPayload)
	}
	browser, auth, err := keys.parse()
	if err != nil {
		return nil, err
	}
	local, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("webpush: key pair: %w", err)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("webpush: salt: %w", err)
	}
	return seal(browser, auth, local, salt, plaintext)
}

// seal is Encrypt with the one-time key pair and the salt given: one
// aes128gcm record behind its header (salt, record size, kwatch's
// one-time public key).
func seal(
	browser *ecdh.PublicKey, auth []byte, local *ecdh.PrivateKey,
	salt, plaintext []byte,
) ([]byte, error) {
	shared, err := local.ECDH(browser)
	if err != nil {
		return nil, fmt.Errorf("webpush: key agreement: %w", err)
	}
	ours := local.PublicKey().Bytes()
	info := append([]byte("WebPush: info\x00"), browser.Bytes()...)
	info = append(info, ours...)
	ikm, err := hkdf.Key(sha256.New, shared, auth, string(info), 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt,
		"Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt,
		"Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 21+len(ours)+len(plaintext)+17)
	out = append(out, salt...)
	out = binary.BigEndian.AppendUint32(out, recordSize)
	out = append(out, byte(len(ours)))
	out = append(out, ours...)
	// One record, so it is the last: the 0x02 delimiter, no padding.
	record := append(append([]byte{}, plaintext...), 2)
	return gcm.Seal(out, nonce, record, nil), nil
}
