package auth

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// region: password-test

func TestHashPasswordRules(t *testing.T) {
	tests := []struct {
		name     string
		password string
		want     error
	}{
		{"14 characters", "abcdefghijklmn", ErrPasswordTooShort},
		{"15 characters", "abcdefghijklmno", nil},
		{"72 bytes", strings.Repeat("a", 72), nil},
		{"73 bytes", strings.Repeat("a", 73), ErrPasswordTooLong},
		{"36 two-byte characters, 72 bytes", strings.Repeat("é", 36), nil},
		{"19 four-byte characters, 76 bytes", strings.Repeat("🔒", 19), ErrPasswordTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := HashPassword(tt.password, bcrypt.MinCost)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if err == nil && !CheckPassword(hash, tt.password) {
				t.Error("CheckPassword rejects the password it hashed")
			}
		})
	}
}

func TestHashLooksLikeBcrypt(t *testing.T) {
	a, _ := HashPassword("correct horse battery", bcrypt.MinCost)
	b, _ := HashPassword("correct horse battery", bcrypt.MinCost)
	// $2a$ names the algorithm, 04$ the cost, then 22 characters of salt
	// and 31 of hash: 60 in all.
	if !strings.HasPrefix(a, "$2a$04$") || len(a) != 60 {
		t.Errorf("hash %q: want $2a$04$ and 60 characters", a)
	}
	if a == b {
		t.Error("two hashes of one password are equal: the salt is missing")
	}
	if CheckPassword(a, "correct horse battery!") || CheckPassword(a, "Correct horse battery") {
		t.Error("CheckPassword accepts a different password")
	}
}

// TestBcryptReadsOnly72Bytes shows why CheckPassword refuses long
// passwords: bcrypt compares only the first 72 bytes, and ignores the rest.
func TestBcryptReadsOnly72Bytes(t *testing.T) {
	first72 := strings.Repeat("a", 72)
	hash, err := bcrypt.GenerateFromPassword([]byte(first72), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := bcrypt.CompareHashAndPassword(hash, []byte(first72+"anything")); err != nil {
		t.Fatalf("bcrypt itself: %v; this test expects it to ignore byte 73 on", err)
	}
	if CheckPassword(string(hash), first72+"anything") {
		t.Error("CheckPassword accepts a password over 72 bytes")
	}
}

// endregion: password-test

func TestDummyHashCost(t *testing.T) {
	if cost, err := bcrypt.Cost([]byte(dummyHash)); err != nil || cost != Cost {
		t.Errorf("dummyHash has cost %d (%v); make a new one with cost %d", cost, err, Cost)
	}
}

// BenchmarkHashPassword shows what one hash costs at each bcrypt cost:
// go test -bench HashPassword ./internal/auth
func BenchmarkHashPassword(b *testing.B) {
	for _, cost := range []int{10, 12, 14} {
		b.Run(fmt.Sprintf("cost=%d", cost), func(b *testing.B) {
			for b.Loop() {
				HashPassword("correct horse battery", cost)
			}
		})
	}
}
