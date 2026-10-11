// Package messaging shows an idempotent consumer: an inbox that remembers
// the ID of every message it has handled, and skips a redelivery.
package messaging

import "sync"

// region: inbox

// Message is one delivery from a queue or a log. The producer sets ID once,
// and every redelivery of the same message carries the same ID.
type Message struct {
	ID   string
	Body []byte
}

// Inbox runs a handler at most once for each message ID that succeeds.
// The zero value is not usable: create one with NewInbox.
type Inbox struct {
	mu     sync.Mutex
	seen   map[string]bool
	handle func(Message) error
}

// NewInbox returns an Inbox that passes each new message to handle.
func NewInbox(handle func(Message) error) *Inbox {
	return &Inbox{seen: map[string]bool{}, handle: handle}
}

// Process runs the handler for m, unless a message with the same ID has
// already succeeded: then it returns duplicate true and does nothing.
// A handler error leaves the ID unseen, and the next delivery of the
// message runs the handler again.
func (in *Inbox) Process(m Message) (duplicate bool, err error) {
	// The lock covers the handler too: two deliveries of one message at
	// the same moment cannot both pass the check before either records it.
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.seen[m.ID] {
		return true, nil
	}
	if err := in.handle(m); err != nil {
		return false, err
	}
	in.seen[m.ID] = true
	return false, nil
}

// endregion: inbox
