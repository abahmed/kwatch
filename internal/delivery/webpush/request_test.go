package webpush

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func TestParseEndpointTakesOnlyPushServices(t *testing.T) {
	for _, ok := range []string{
		"https://fcm.googleapis.com/fcm/send/abc:def",
		"https://updates.push.services.mozilla.com/wpush/v2/gAAA",
		"https://web.push.apple.com/QGt1",
		"https://wns2-par02p.notify.windows.com/w/?token=AQE",
		"https://FCM.googleapis.com/fcm/send/x",
	} {
		if _, err := ParseEndpoint(ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://fcm.googleapis.com/fcm/send/x",
		"https://fcm.googleapis.com:8443/fcm/send/x",
		"https://user:pw@fcm.googleapis.com/fcm/send/x",
		"https://fcm.googleapis.com.evil.example/x",
		"https://169.254.169.254/latest/meta-data",
		"https://kubernetes.default.svc/api",
		"https://push.apple.com/x",
		"https://evilnotify.windows.com/x",
		"https://fcm.googleapis.com/" + strings.Repeat("a", maxEndpoint),
		"fcm.googleapis.com/fcm/send/x",
		"",
	} {
		if _, err := ParseEndpoint(bad); !errors.Is(err, ErrEndpoint) {
			t.Errorf("%q taken", bad)
		}
	}
}

func TestKeySurvivesMarshal(t *testing.T) {
	k, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	s, err := k.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseKey(s)
	if err != nil || back.Public() != k.Public() {
		t.Fatalf("ParseKey = %v, %v", back.Public(), err)
	}
	if raw, _ := decode(k.Public()); len(raw) != 65 || raw[0] != 4 {
		t.Fatalf("public key is not an uncompressed point: %d", len(raw))
	}
	if _, err := ParseKey("AAAA"); err == nil {
		t.Fatal("ParseKey took a short key")
	}
}

func TestRequestIsSignedAndEncrypted(t *testing.T) {
	k, _ := NewKey()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	sub := Subscription{
		Endpoint: "https://fcm.googleapis.com/fcm/send/abc",
		Keys:     Keys{P256dh: rfcUAPublic, Auth: rfcAuth},
	}
	r, err := Request(sub, []byte(`{"t":1}`), k, now, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if r.URL != sub.Endpoint || r.Method != http.MethodPost ||
		r.Headers["TTL"] != "3600" || r.Headers["Urgency"] != "high" ||
		r.Headers["Content-Encoding"] != "aes128gcm" {
		t.Fatalf("request = %+v", r)
	}
	auth := r.Headers["Authorization"]
	token, pub, ok := strings.Cut(strings.TrimPrefix(auth, "vapid t="),
		", k=")
	if !ok || pub != k.Public() {
		t.Fatalf("Authorization = %q", auth)
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("token = %q", token)
	}
	var claims struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}
	raw, _ := decode(parts[1])
	if err := json.Unmarshal(raw, &claims); err != nil ||
		claims.Aud != "https://fcm.googleapis.com" ||
		claims.Exp != now.Add(12*time.Hour).Unix() ||
		claims.Sub != Subject {
		t.Fatalf("claims = %+v, %v", claims, err)
	}
	sig, _ := decode(parts[2])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r1 := new(big.Int).SetBytes(sig[:32])
	s1 := new(big.Int).SetBytes(sig[32:])
	if len(sig) != 64 || !ecdsa.Verify(&k.private.PublicKey, digest[:],
		r1, s1) {
		t.Fatal("signature does not verify")
	}
	if _, err := Request(Subscription{Endpoint: "https://example.com/x",
		Keys: sub.Keys}, nil, k, now, time.Hour); err == nil {
		t.Fatal("request to an unknown host built")
	}
}

func TestGoneMeansTheSubscriptionEnded(t *testing.T) {
	for code, want := range map[int]bool{404: true, 410: true,
		429: false, 500: false, 413: false} {
		err := &transport.StatusError{StatusCode: code}
		if Gone(err) != want {
			t.Errorf("Gone(%d) = %v", code, !want)
		}
	}
	if Gone(errors.New("dial tcp: timeout")) {
		t.Error("a network error counted as gone")
	}
}
