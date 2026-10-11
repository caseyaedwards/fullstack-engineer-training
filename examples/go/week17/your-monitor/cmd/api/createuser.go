package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"fsp/examples/week17/your-monitor/internal/auth"
)

// region: create-user

// createUser reads the password from the first line of in and adds a user
// with it. The password never sits on the command line, where ps shows it
// to every user of the machine and the shell writes it to its history.
func createUser(ctx context.Context, svc *auth.Service, email string, in io.Reader, out io.Writer) error {
	line, err := bufio.NewReader(in).ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		return errors.New("no password on stdin")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	u, err := svc.CreateUser(ctx, email, password)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "created user id=%d email=%s\n", u.ID, u.Email)
	return nil
}

// isTerminal reports whether f is a terminal, where a typed password
// shows on the screen. It checks for a character device, which /dev/null
// is too: an empty stdin has no password either.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// endregion: create-user
