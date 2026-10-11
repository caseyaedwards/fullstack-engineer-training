// Package networking is a line echo server and its client over TCP,
// with deadlines on both sides.
package networking

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// idleTimeout is how long the server waits for the next line before it
// closes the connection.
const idleTimeout = 2 * time.Second

// region: serve

// Serve accepts connections on ln and echoes each line back on the same
// connection. It returns nil once ln is closed, and the first other
// Accept error otherwise.
func Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		if err != nil {
			return err
		}
		go handle(conn)
	}
}

func handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		// A client that sends nothing for idleTimeout loses the
		// connection, and its file descriptor goes back to the kernel.
		conn.SetReadDeadline(time.Now().Add(idleTimeout))
		line, err := r.ReadString('\n')
		if err != nil {
			return // the client closed, or the deadline passed
		}
		if _, err := conn.Write([]byte(line)); err != nil {
			return
		}
	}
}

// endregion: serve

// region: echo

// Echo connects to addr, sends line with a newline, and returns the line
// the server sends back. timeout bounds the whole exchange: the connect,
// the write and the read.
func Echo(addr, line string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return "", fmt.Errorf("echo: %w", err)
	}
	defer conn.Close()
	conn.SetDeadline(deadline)

	if _, err := fmt.Fprintf(conn, "%s\n", line); err != nil {
		return "", fmt.Errorf("echo: write: %w", err)
	}
	reply, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("echo: read: %w", err)
	}
	return strings.TrimSuffix(reply, "\n"), nil
}

// endregion: echo
