package auth

import (
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

// region: password

// Cost is the bcrypt cost of new hashes: bcrypt runs 2^Cost rounds. Each
// step up doubles the time a hash takes, for you and for an attacker.
const Cost = 12

// The password rules. bcrypt reads only the first 72 bytes of a password,
// and a character outside ASCII takes 2 to 4 bytes in UTF-8.
const (
	MinPasswordChars = 12
	MaxPasswordBytes = 72
)

var (
	ErrPasswordTooShort = fmt.Errorf("password must have at least %d characters", MinPasswordChars)
	ErrPasswordTooLong  = fmt.Errorf("password must be at most %d bytes", MaxPasswordBytes)
)

// HashPassword checks the password rules and returns a bcrypt hash with a
// new random salt, such as $2a$12$ followed by 53 characters.
func HashPassword(password string, cost int) (string, error) {
	if utf8.RuneCountInString(password) < MinPasswordChars {
		return "", ErrPasswordTooShort
	}
	if len(password) > MaxPasswordBytes { // len counts bytes, not characters
		return "", ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return string(hash), err
}

// CheckPassword reports whether password matches hash. It refuses a
// password over 72 bytes, which bcrypt compares by its first 72 bytes
// alone.
func CheckPassword(hash, password string) bool {
	if len(password) > MaxPasswordBytes {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// endregion: password
