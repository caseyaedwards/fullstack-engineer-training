package operatingsystems

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// region: test
func TestRunReportsTheExitCode(t *testing.T) {
	code, err := Run(context.Background(), 5*time.Second, "sh", "-c", "exit 3")
	if err != nil {
		t.Fatalf("Run: %v, want nil: a non-zero exit is a result, not an error", err)
	}
	if code != 3 {
		t.Errorf("exit code = %d, want 3", code)
	}
}

func TestRunStopsAtTheTimeout(t *testing.T) {
	start := time.Now()
	_, err := Run(context.Background(), 100*time.Millisecond, "sleep", "10")
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("Run took %v, want well under 2s for a 100ms timeout", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), "signal: terminated") {
		t.Errorf("err = %v, want the child ended by SIGTERM", err)
	}
}

// endregion: test

func TestRunSuccess(t *testing.T) {
	code, err := Run(context.Background(), 5*time.Second, "true")
	if err != nil || code != 0 {
		t.Errorf("Run(true) = %d, %v; want 0, nil", code, err)
	}
}

// A child that ignores SIGTERM gets SIGKILL once WaitDelay runs out.
// The disposition "ignore" survives exec, which makes sleep itself
// ignore SIGTERM, with no shell left behind. The 500ms timeout gives sh
// time to run trap before SIGTERM arrives, even on a busy CI runner.
func TestRunKillsAChildThatIgnoresSIGTERM(t *testing.T) {
	start := time.Now()
	_, err := Run(context.Background(), 500*time.Millisecond, "sh", "-c", `trap "" TERM; exec sleep 10`)
	elapsed := time.Since(start)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("err = %v, want the child ended by SIGKILL", err)
	}
	if elapsed < time.Second || elapsed > 5*time.Second {
		t.Errorf("Run took %v, want the 1s WaitDelay after the 500ms timeout, under 5s", elapsed)
	}
}

// A child that a signal ends, here a SIGKILL it sends itself, as the
// out-of-memory killer does, has no exit code: Run reports an error.
func TestRunReportsAChildEndedByASignal(t *testing.T) {
	code, err := Run(context.Background(), 5*time.Second, "sh", "-c", "kill -KILL $$")
	if code != -1 {
		t.Errorf("exit code = %d, want -1", code)
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want an *exec.ExitError", err)
	}
	if !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("err = %v, want signal: killed", err)
	}
}

func TestRunMissingProgram(t *testing.T) {
	_, err := Run(context.Background(), time.Second, "no-such-program-fsp")
	if !errors.Is(err, exec.ErrNotFound) {
		t.Errorf("err = %v, want exec.ErrNotFound", err)
	}
}

// Cancelling the parent context stops the child the same way, with
// context.Canceled instead of DeadlineExceeded.
func TestRunHonorsTheParentContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := Run(ctx, time.Minute, "sleep", "10")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
