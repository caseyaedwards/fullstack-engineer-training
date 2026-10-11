// Package timentp computes clock offset and network delay from the four
// timestamps of one NTP exchange.
package timentp

import "time"

// region: offset

// Offset returns how far the server clock runs ahead of the client clock,
// and the round-trip delay on the network, from one exchange:
//
//	t1: the client sends the request, by the client clock
//	t2: the server receives it, by the server clock
//	t3: the server sends the reply, by the server clock
//	t4: the client receives the reply, by the client clock
//
// A negative offset means the server clock is behind. The result assumes
// the request and the reply spend the same time on the network.
func Offset(t1, t2, t3, t4 time.Time) (offset, delay time.Duration) {
	offset = (t2.Sub(t1) + t3.Sub(t4)) / 2
	delay = t4.Sub(t1) - t3.Sub(t2)
	return offset, delay
}

// endregion: offset
