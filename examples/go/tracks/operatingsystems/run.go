// Package operatingsystems runs a child process with a deadline: SIGTERM
// first, SIGKILL when the child does not exit in time.
package operatingsystems

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// region: run

// Run starts name with args as a child process and waits for it to exit.
// A non-zero exit is a result: Run returns the code and a nil error.
// When timeout passes or ctx ends first, Run sends the child SIGTERM,
// waits up to one second more, then sends SIGKILL, and returns an error
// that wraps the context error.
func Run(ctx context.Context, timeout time.Duration, name string, args ...string) (exitCode int, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	// The default Cancel sends SIGKILL. Ask the child to exit first.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	// After Cancel, Wait gives the child this long, then kills it.
	cmd.WaitDelay = time.Second

	err = cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return -1, fmt.Errorf("run %s: %w: %v", name, ctxErr, err)
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if err != nil {
		return -1, fmt.Errorf("run %s: %w", name, err) // it never started
	}
	return 0, nil
}

// endregion: run
