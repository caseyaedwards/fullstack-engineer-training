package main

// region: limits

import (
	"flag"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/time/rate"

	"fsp/examples/week18/your-monitor/internal/api"
	"fsp/examples/week18/your-monitor/internal/ratelimit"
)

// limitFlags declares the rate-limit flags on fs. Each one takes its
// default from an environment variable, which a Kubernetes Deployment can
// set without touching the command line.
func limitFlags(fs *flag.FlagSet) func() (api.RateConfig, error) {
	limit := fs.Float64("rate-limit", envFloat("RATE_LIMIT", 10), "requests per second allowed per client address; 0 turns the limit off (env RATE_LIMIT)")
	burst := fs.Int("rate-burst", int(envFloat("RATE_BURST", 40)), "requests a client can send at once before the limit applies (env RATE_BURST)")
	trusted := fs.String("trusted-proxies", os.Getenv("TRUSTED_PROXIES"), "comma-separated networks of the proxies in front of the api, such as 10.42.0.0/16; their X-Forwarded-For is believed (env TRUSTED_PROXIES)")
	return func() (api.RateConfig, error) {
		nets, err := ratelimit.ParsePrefixes(*trusted)
		if err != nil {
			return api.RateConfig{}, err
		}
		if *limit < 0 || (*limit > 0 && *burst < 1) {
			return api.RateConfig{}, fmt.Errorf("-rate-limit must be 0 or more, and -rate-burst at least 1")
		}
		return api.RateConfig{Limit: rate.Limit(*limit), Burst: *burst, Trusted: nets}, nil
	}
}

// envFloat returns the number in the environment variable name, or def
// when it is unset or not a number.
func envFloat(name string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil {
		return v
	}
	return def
}

// endregion: limits
