// Package dns sends one DNS question for an A record over UDP and reads
// the answer, with the message format from golang.org/x/net/dns/dnsmessage.
package dns

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"strings"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// region: query

// ErrNXDomain means the name does not exist: response code 3, NXDOMAIN.
var ErrNXDomain = errors.New("dns: no such domain")

// BuildQuery packs a question for the A record of name, with the given
// message ID and the recursion desired bit set, as a stub resolver sends it.
func BuildQuery(id uint16, name string) ([]byte, error) {
	if !strings.HasSuffix(name, ".") {
		name += "." // a fully qualified name ends at the root
	}
	n, err := dnsmessage.NewName(name)
	if err != nil {
		return nil, fmt.Errorf("dns: %w", err)
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, RecursionDesired: true})
	if err := b.StartQuestions(); err != nil {
		return nil, err
	}
	q := dnsmessage.Question{Name: n, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET}
	if err := b.Question(q); err != nil {
		return nil, err
	}
	return b.Finish()
}

// endregion: query

// region: parse

// ParseA reads a reply and returns every A record in its answer section,
// with the lowest TTL among the answers: the number of seconds the whole
// answer, CNAME records included, stays valid in a cache.
func ParseA(msg []byte) ([]netip.Addr, uint32, error) {
	var p dnsmessage.Parser
	h, err := p.Start(msg)
	if err != nil {
		return nil, 0, fmt.Errorf("dns: %w", err)
	}
	switch h.RCode {
	case dnsmessage.RCodeSuccess:
	case dnsmessage.RCodeNameError:
		return nil, 0, ErrNXDomain
	default:
		return nil, 0, fmt.Errorf("dns: server answered %v", h.RCode)
	}
	if err := p.SkipAllQuestions(); err != nil {
		return nil, 0, fmt.Errorf("dns: %w", err)
	}
	var addrs []netip.Addr
	var ttl uint32
	for i := 0; ; i++ {
		rh, err := p.AnswerHeader()
		if errors.Is(err, dnsmessage.ErrSectionDone) {
			break
		}
		if err != nil {
			return nil, 0, fmt.Errorf("dns: %w", err)
		}
		if i == 0 || rh.TTL < ttl {
			ttl = rh.TTL
		}
		if rh.Type != dnsmessage.TypeA {
			if err := p.SkipAnswer(); err != nil { // a CNAME, for example
				return nil, 0, fmt.Errorf("dns: %w", err)
			}
			continue
		}
		a, err := p.AResource()
		if err != nil {
			return nil, 0, fmt.Errorf("dns: %w", err)
		}
		addrs = append(addrs, netip.AddrFrom4(a.A))
	}
	return addrs, ttl, nil
}

// endregion: parse

// region: lookup

// Lookup asks server, a host:port such as 1.1.1.1:53, for the A records
// of name over UDP. It waits until ctx ends, or five seconds when ctx has
// no deadline, and drops any reply whose ID does not match the question.
// When ctx ends first, the error wraps ctx.Err().
func Lookup(ctx context.Context, server, name string) ([]netip.Addr, uint32, error) {
	id := uint16(rand.Uint32())
	query, err := BuildQuery(id, name)
	if err != nil {
		return nil, 0, err
	}

	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", server)
	if err != nil {
		return nil, 0, fmt.Errorf("dns: %w", err)
	}
	defer conn.Close()
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(5 * time.Second)
	}
	conn.SetDeadline(deadline)
	// A cancel without a deadline ends the wait too.
	stop := context.AfterFunc(ctx, func() { conn.SetDeadline(time.Now()) })
	defer stop()

	if _, err := conn.Write(query); err != nil {
		return nil, 0, fmt.Errorf("dns: %w", err)
	}
	buf := make([]byte, 512) // the UDP limit for a DNS message without EDNS
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if ctxErr := contextErr(ctx); ctxErr != nil {
				return nil, 0, fmt.Errorf("dns: %w: %w", ctxErr, err)
			}
			return nil, 0, fmt.Errorf("dns: %w", err)
		}
		var p dnsmessage.Parser
		h, err := p.Start(buf[:n])
		if err != nil || !h.Response || h.ID != id {
			continue // not the answer to this question
		}
		if h.Truncated {
			return nil, 0, errors.New("dns: answer truncated, ask again over TCP")
		}
		return ParseA(buf[:n])
	}
}

// contextErr returns ctx.Err(), or context.DeadlineExceeded once the ctx
// deadline has passed: the socket deadline and the context timer fire at
// the same instant, and the socket can report first.
func contextErr(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d, ok := ctx.Deadline(); ok && !time.Now().Before(d) {
		return context.DeadlineExceeded
	}
	return nil
}

// endregion: lookup
