// Command jwt makes and checks the tokens of the Week 11 side lab. It
// reads the signing key from JWT_KEY. Run it from examples/go:
//
//	JWT_KEY=... go run ./week11/cmd/jwt -sub you@example.com
//	JWT_KEY=... go run ./week11/cmd/jwt -verify TOKEN
package main

// region: main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"fsp/examples/week11"
)

func main() {
	sub := flag.String("sub", "", "make a token for this subject")
	ttl := flag.Duration("ttl", time.Hour, "how long the token lasts")
	verify := flag.String("verify", "", "check this token and print its claims")
	flag.Parse()

	key := []byte(os.Getenv("JWT_KEY"))
	if len(key) < 32 {
		fmt.Fprintln(os.Stderr, "set JWT_KEY to at least 32 bytes, for example: export JWT_KEY=$(openssl rand -base64 32)")
		os.Exit(2)
	}
	switch {
	case *verify != "":
		c, err := week11.Verify(*verify, key, time.Now())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("valid: sub=%s expires=%s\n", c.Subject, time.Unix(c.ExpiresAt, 0).UTC().Format(time.RFC3339))
	case *sub != "":
		token, err := week11.Sign(week11.Claims{Subject: *sub, ExpiresAt: time.Now().Add(*ttl).Unix()}, key)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(token)
	default:
		flag.Usage()
		os.Exit(2)
	}
}

// endregion: main
