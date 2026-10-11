package dns

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// answer builds the reply a fake server sends to query: the same ID and
// question, the given response code, and one A record per address.
func answer(t *testing.T, query []byte, id uint16, rcode dnsmessage.RCode, ttl uint32, addrs ...string) []byte {
	t.Helper()
	var p dnsmessage.Parser
	if _, err := p.Start(query); err != nil {
		t.Errorf("fake server: parse query: %v", err)
		return nil
	}
	q, err := p.Question()
	if err != nil {
		t.Errorf("fake server: read question: %v", err)
		return nil
	}
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: id, Response: true, RecursionAvailable: true, RCode: rcode})
	b.StartQuestions()
	b.Question(q)
	b.StartAnswers()
	for _, a := range addrs {
		ip := netip.MustParseAddr(a).As4()
		h := dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: ttl}
		if err := b.AResource(h, dnsmessage.AResource{A: ip}); err != nil {
			t.Errorf("fake server: %v", err)
			return nil
		}
	}
	msg, err := b.Finish()
	if err != nil {
		t.Errorf("fake server: %v", err)
	}
	return msg
}

// fakeServer answers DNS queries on a free UDP port of 127.0.0.1. reply
// turns each query and its ID into the datagrams to send back.
func fakeServer(t *testing.T, reply func(query []byte, id uint16) [][]byte) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pc.Close() })
	go func() {
		buf := make([]byte, 512)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return // closed by the cleanup
			}
			query := append([]byte(nil), buf[:n]...)
			var p dnsmessage.Parser
			h, err := p.Start(query)
			if err != nil {
				continue
			}
			for _, msg := range reply(query, h.ID) {
				pc.WriteTo(msg, from)
			}
		}
	}()
	return pc.LocalAddr().String()
}

// region: test
func TestLookupReturnsAddressAndTTL(t *testing.T) {
	server := fakeServer(t, func(q []byte, id uint16) [][]byte {
		return [][]byte{answer(t, q, id, dnsmessage.RCodeSuccess, 300, "203.0.113.10")}
	})
	addrs, ttl, err := Lookup(context.Background(), server, "example.com")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	want := netip.MustParseAddr("203.0.113.10")
	if len(addrs) != 1 || addrs[0] != want {
		t.Errorf("addrs = %v, want [%v]", addrs, want)
	}
	if ttl != 300 {
		t.Errorf("ttl = %d, want 300", ttl)
	}
}

func TestLookupNXDomain(t *testing.T) {
	server := fakeServer(t, func(q []byte, id uint16) [][]byte {
		return [][]byte{answer(t, q, id, dnsmessage.RCodeNameError, 0)}
	})
	_, _, err := Lookup(context.Background(), server, "no-such-host.example")
	if !errors.Is(err, ErrNXDomain) {
		t.Fatalf("err = %v, want ErrNXDomain", err)
	}
}

// endregion: test

// A reply with the wrong ID does not answer this query: Lookup drops it
// and waits for the real one, which blocks the simplest spoofing attack.
func TestLookupIgnoresAReplyWithTheWrongID(t *testing.T) {
	server := fakeServer(t, func(q []byte, id uint16) [][]byte {
		return [][]byte{
			answer(t, q, id+1, dnsmessage.RCodeSuccess, 300, "192.0.2.66"),
			answer(t, q, id, dnsmessage.RCodeSuccess, 300, "203.0.113.10"),
		}
	})
	addrs, _, err := Lookup(context.Background(), server, "example.com")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if len(addrs) != 1 || addrs[0] != netip.MustParseAddr("203.0.113.10") {
		t.Errorf("addrs = %v, want only 203.0.113.10", addrs)
	}
}

// A server that never answers: Lookup gives up when ctx does.
func TestLookupHonorsTheContext(t *testing.T) {
	server := fakeServer(t, func([]byte, uint16) [][]byte { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, _, err := Lookup(ctx, server, "example.com")
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Lookup took %v, want well under 2s for a 100ms timeout", elapsed)
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Errorf("err = %v, want os.ErrDeadlineExceeded", err)
	}
}

func TestBuildQuery(t *testing.T) {
	msg, err := BuildQuery(0xbeef, "example.com")
	if err != nil {
		t.Fatal(err)
	}
	var p dnsmessage.Parser
	h, err := p.Start(msg)
	if err != nil {
		t.Fatal(err)
	}
	if h.ID != 0xbeef || h.Response || !h.RecursionDesired {
		t.Errorf("header = %+v, want ID 0xbeef, a query, recursion desired", h)
	}
	q, err := p.Question()
	if err != nil {
		t.Fatal(err)
	}
	if q.Name.String() != "example.com." || q.Type != dnsmessage.TypeA || q.Class != dnsmessage.ClassINET {
		t.Errorf("question = %v, want example.com. A IN", q)
	}
}

// The walkthrough on the track page: www.example.com is a CNAME for
// example.com, which has the A record. ParseA follows the answer section
// and reports the lowest TTL in the chain.
func TestParseAFollowsACNAMEChain(t *testing.T) {
	www := dnsmessage.MustNewName("www.example.com.")
	apex := dnsmessage.MustNewName("example.com.")
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 1, Response: true})
	b.StartQuestions()
	b.Question(dnsmessage.Question{Name: www, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET})
	b.StartAnswers()
	b.CNAMEResource(dnsmessage.ResourceHeader{Name: www, Type: dnsmessage.TypeCNAME, Class: dnsmessage.ClassINET, TTL: 3600},
		dnsmessage.CNAMEResource{CNAME: apex})
	b.AResource(dnsmessage.ResourceHeader{Name: apex, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 300},
		dnsmessage.AResource{A: [4]byte{203, 0, 113, 10}})
	msg, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	addrs, ttl, err := ParseA(msg)
	if err != nil {
		t.Fatal(err)
	}
	if len(addrs) != 1 || addrs[0] != netip.MustParseAddr("203.0.113.10") || ttl != 300 {
		t.Errorf("ParseA = %v, %d; want [203.0.113.10], 300", addrs, ttl)
	}
}

// A name with no A record answers NOERROR with an empty answer section:
// no addresses and no error, which is not the same as NXDOMAIN.
func TestParseANoData(t *testing.T) {
	b := dnsmessage.NewBuilder(nil, dnsmessage.Header{ID: 1, Response: true})
	msg, err := b.Finish()
	if err != nil {
		t.Fatal(err)
	}
	addrs, _, err := ParseA(msg)
	if err != nil || len(addrs) != 0 {
		t.Errorf("ParseA = %v, %v; want no addresses and no error", addrs, err)
	}
}
