// Package week11 holds the side lab of Week 11: a JSON Web Token (JWT)
// made and checked with the standard library, to compare with the
// session cookie the uptime monitor uses. The monitor itself lives in
// your-monitor/.
package week11

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// region: sign

// Claims is the payload of the token: who it is for and when it ends.
type Claims struct {
	Subject   string `json:"sub"`
	ExpiresAt int64  `json:"exp"` // Unix time, in seconds
}

var b64 = base64.RawURLEncoding // base64url without = padding, as JWT uses

// header is the same for every token here: HMAC with SHA-256.
var header = b64.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

// Sign returns header.payload.signature. The payload is only encoded,
// not encrypted: anyone who holds the token can read it.
func Sign(c Claims, key []byte) (string, error) {
	payload, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	unsigned := header + "." + b64.EncodeToString(payload)
	return unsigned + "." + b64.EncodeToString(mac(unsigned, key)), nil
}

func mac(unsigned string, key []byte) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(unsigned))
	return m.Sum(nil)
}

// endregion: sign

// region: verify

var (
	ErrMalformed    = errors.New("token is not header.payload.signature")
	ErrBadSignature = errors.New("signature does not match")
	ErrExpired      = errors.New("token expired")
)

// Verify checks the signature and the expiry, and returns the claims. It
// needs no database: the key and the clock are enough. That is the
// strength of a token, and also why nothing can end one early.
func Verify(token string, key []byte, now time.Time) (Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != header { // refuses "alg":"none" and every other header
		return Claims{}, ErrMalformed
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	// hmac.Equal takes as long for a near miss as for a wild guess.
	if !hmac.Equal(sig, mac(parts[0]+"."+parts[1], key)) {
		return Claims{}, ErrBadSignature
	}
	payload, err := b64.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	var c Claims
	if err := json.Unmarshal(payload, &c); err != nil {
		return Claims{}, ErrMalformed
	}
	if now.Unix() >= c.ExpiresAt {
		return Claims{}, ErrExpired
	}
	return c, nil
}

// endregion: verify
