package main

import (
	"bytes"
	"log"
	"os"
	"testing"

	"fsp/examples/week09/your-monitor/internal/alert"
)

func TestNewNotifier(t *testing.T) {
	var logs bytes.Buffer
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	if _, ok := newNotifier("").(alert.Log); !ok {
		t.Error("an empty ALERT_WEBHOOK_URL must give the log notifier")
	}
	if !bytes.Contains(logs.Bytes(), []byte("ALERT_WEBHOOK_URL is empty")) {
		t.Errorf("no warning about the missing webhook in the log: %q", logs.String())
	}
	n, ok := newNotifier("https://hooks.example/abc").(alert.Webhook)
	if !ok || n.URL != "https://hooks.example/abc" {
		t.Errorf("newNotifier(url) = %#v, want a webhook notifier for that URL", n)
	}
}
