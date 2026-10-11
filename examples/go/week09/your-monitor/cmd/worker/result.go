// contract stub: week 08
//
// The Week 8 worker turns the output of probe.Check into one row of
// check_results. Week 9 keeps that conversion as it is.

package main

import (
	"time"

	"fsp/examples/week09/your-monitor/internal/monitor"
	"fsp/examples/week09/your-monitor/internal/probe"
)

// toResult builds the check_results row for one check that started at at.
func toResult(monitorID int64, at time.Time, t probe.Timings, status int, err error) monitor.Result {
	ms := func(d time.Duration) int { return int(d.Milliseconds()) }
	r := monitor.Result{
		MonitorID: monitorID, CheckedAt: at, Up: err == nil && status < 400, StatusCode: status,
		DNSMs: ms(t.DNS), ConnectMs: ms(t.Connect), TLSMs: ms(t.TLS), FirstByteMs: ms(t.FirstByte), TotalMs: ms(t.Total),
	}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}
