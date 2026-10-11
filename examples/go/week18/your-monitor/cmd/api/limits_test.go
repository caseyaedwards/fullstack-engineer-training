package main

import (
	"flag"
	"testing"
)

// TestLimitFlags: the environment sets the defaults, and a flag on the
// command line wins over the environment.
func TestLimitFlags(t *testing.T) {
	t.Setenv("RATE_LIMIT", "5")
	t.Setenv("TRUSTED_PROXIES", "10.42.0.0/16")

	fs := flag.NewFlagSet("api", flag.ContinueOnError)
	config := limitFlags(fs)
	if err := fs.Parse([]string{"-rate-burst", "20"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := config()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Limit != 5 || cfg.Burst != 20 || len(cfg.Trusted) != 1 || cfg.Trusted[0].String() != "10.42.0.0/16" {
		t.Errorf("got %+v, want limit 5, burst 20, trusted 10.42.0.0/16", cfg)
	}

	for _, args := range [][]string{
		{"-trusted-proxies", "traefik"},
		{"-rate-limit", "-1"},
		{"-rate-burst", "0"},
	} {
		fs := flag.NewFlagSet("api", flag.ContinueOnError)
		config := limitFlags(fs)
		if err := fs.Parse(args); err != nil {
			t.Fatal(err)
		}
		if _, err := config(); err == nil {
			t.Errorf("%v: got nil, want an error", args)
		}
	}
}
