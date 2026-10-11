package week11

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

var (
	key = []byte("a key of at least 32 bytes, from the environment")
	now = time.Date(2026, 10, 11, 9, 0, 0, 0, time.UTC)
)

// region: jwt-test

func TestJWT(t *testing.T) {
	token, err := Sign(Claims{Subject: "you@example.com", ExpiresAt: now.Add(time.Hour).Unix()}, key)
	if err != nil {
		t.Fatal(err)
	}
	if c, err := Verify(token, key, now); err != nil || c.Subject != "you@example.com" {
		t.Fatalf("Verify = %+v, %v", c, err)
	}

	// Anyone can read the payload: it is base64url, not encryption.
	payload, _ := base64.RawURLEncoding.DecodeString(strings.Split(token, ".")[1])
	if string(payload) != `{"sub":"you@example.com","exp":1791712800}` {
		t.Errorf("payload %s", payload)
	}

	// Changing the payload breaks the signature.
	parts := strings.Split(token, ".")
	forged := parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"admin@example.com","exp":1791712800}`)) + "." + parts[2]
	if _, err := Verify(forged, key, now); !errors.Is(err, ErrBadSignature) {
		t.Errorf("forged payload: err = %v, want ErrBadSignature", err)
	}
	// So does a different key.
	if _, err := Verify(token, []byte("another key"), now); !errors.Is(err, ErrBadSignature) {
		t.Errorf("wrong key: err = %v, want ErrBadSignature", err)
	}
	// An unsigned token with "alg":"none" is refused.
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + parts[1] + "."
	if _, err := Verify(none, key, now); !errors.Is(err, ErrMalformed) {
		t.Errorf(`"alg":"none": err = %v, want ErrMalformed`, err)
	}
	// Nothing ends the token early. Only the clock does.
	if _, err := Verify(token, key, now.Add(59*time.Minute)); err != nil {
		t.Errorf("59 minutes later: %v", err)
	}
	if _, err := Verify(token, key, now.Add(time.Hour)); !errors.Is(err, ErrExpired) {
		t.Errorf("an hour later: err = %v, want ErrExpired", err)
	}
}

// endregion: jwt-test
