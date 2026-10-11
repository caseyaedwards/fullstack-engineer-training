// Package alert turns check results into incidents and sends a
// notification when an incident opens or resolves.
package alert

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"
)

// region: notifier

// Kind says what happened to an incident.
type Kind string

const (
	Opened   Kind = "opened"
	Resolved Kind = "resolved"
)

// Event is one notification: an incident opened or resolved.
type Event struct {
	Kind       Kind      `json:"kind"`
	IncidentID int64     `json:"incident_id"`
	MonitorID  int64     `json:"monitor_id"`
	URL        string    `json:"url"`
	Cause      string    `json:"cause"`
	At         time.Time `json:"at"`
}

// Notifier sends one event to people. Notify returns an error when the
// event did not arrive, and the alerter then tries again on its next round.
type Notifier interface {
	Notify(ctx context.Context, e Event) error
}

// endregion: notifier

// region: webhook

// Webhook sends each event as a JSON POST to URL. The URL of a chat
// webhook works as a password: anyone who has it can post to the channel.
// Keep it in the environment, never in the repository or in an error.
type Webhook struct {
	URL    string
	Client *http.Client // nil means a client with a 5-second timeout
}

func (w Webhook) Notify(ctx context.Context, e Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhook: bad ALERT_WEBHOOK_URL")
	}
	req.Header.Set("Content-Type", "application/json")

	client := w.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		// A *url.Error quotes the full URL, secret path included. Keep
		// the host and the cause, such as a timeout, and drop the rest.
		var uerr *url.Error
		if errors.As(err, &uerr) {
			err = uerr.Err
		}
		return fmt.Errorf("webhook to %s: %w", req.URL.Host, err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhook to %s: status %d", req.URL.Host, resp.StatusCode)
	}
	return nil
}

// endregion: webhook

// region: log

// Log writes each event as one line to Logger. Use it on your laptop, and
// as the fallback when ALERT_WEBHOOK_URL is empty.
type Log struct {
	Logger *log.Logger
}

func (l Log) Notify(ctx context.Context, e Event) error {
	l.Logger.Printf("alert %s incident=%d monitor=%d url=%s cause=%q",
		e.Kind, e.IncidentID, e.MonitorID, e.URL, e.Cause)
	return nil
}

// endregion: log
