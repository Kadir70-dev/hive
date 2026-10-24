// Package mail renders and delivers transactional email through a pluggable
// provider. Delivery normally goes through the Postgres outbox (outbox.go) so
// an email is never lost when the provider is briefly unavailable.
package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Message is one rendered email.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Sender delivers a message. IdempotencyKey lets providers drop duplicates
// when a retry follows a send whose response was lost.
type Sender interface {
	Send(ctx context.Context, msg Message, idempotencyKey string) (providerID string, err error)
}

// PermanentError marks failures that retrying cannot fix (bad address, auth).
type PermanentError struct{ Err error }

func (e *PermanentError) Error() string { return e.Err.Error() }
func (e *PermanentError) Unwrap() error { return e.Err }

// IsPermanent reports whether err should not be retried.
func IsPermanent(err error) bool {
	var p *PermanentError
	return errors.As(err, &p)
}

// Resend sends through https://resend.com's HTTP API.
type Resend struct {
	APIKey  string
	From    string // e.g. "The Hive Society <hello@mailer.thehivesociety.ae>"
	ReplyTo string // e.g. hello@thehivesociety.ae (Titan inbox; no DNS change)
	BaseURL string // defaults to https://api.resend.com
	Client  *http.Client
}

// NewResend validates configuration.
func NewResend(apiKey, from, replyTo string) (*Resend, error) {
	if !strings.HasPrefix(apiKey, "re_") {
		return nil, errors.New("RESEND_API_KEY missing or malformed")
	}
	if !strings.Contains(from, "@") {
		return nil, errors.New("MAIL_FROM missing")
	}
	return &Resend{APIKey: apiKey, From: from, ReplyTo: replyTo, BaseURL: "https://api.resend.com",
		Client: &http.Client{Timeout: 15 * time.Second}}, nil
}

// Send implements Sender.
func (r *Resend) Send(ctx context.Context, msg Message, idempotencyKey string) (string, error) {
	body := map[string]any{"from": r.From, "to": []string{msg.To}, "subject": msg.Subject, "text": msg.Text, "html": msg.HTML}
	if r.ReplyTo != "" {
		body["reply_to"] = r.ReplyTo
	}
	payload, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(r.BaseURL, "/")+"/emails", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	resp, err := r.Client.Do(req)
	if err != nil {
		return "", fmt.Errorf("resend: request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		var out struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(raw, &out)
		return out.ID, nil
	}
	// Provider error bodies describe the request, not the recipient; keep them short.
	msgErr := fmt.Errorf("resend: HTTP %d: %.200s", resp.StatusCode, strings.TrimSpace(string(raw)))
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return "", msgErr
	}
	return "", &PermanentError{Err: msgErr}
}

// Log is a development sender. It never contacts a provider. Bodies (which
// contain one-time links) are only written when LogBodies is set.
type Log struct {
	Logger    *slog.Logger
	LogBodies bool
}

// Send implements Sender.
func (l *Log) Send(_ context.Context, msg Message, key string) (string, error) {
	if l.LogBodies {
		l.Logger.Info("mail (log provider)", "to", msg.To, "subject", msg.Subject, "text", msg.Text)
	} else {
		l.Logger.Info("mail (log provider)", "subject", msg.Subject, "idempotency_key", key)
	}
	return "log-" + key, nil
}

// Recorder captures messages in memory; used by tests.
type Recorder struct {
	mu   sync.Mutex
	Sent []Message
	Fail error
}

// Send implements Sender.
func (r *Recorder) Send(_ context.Context, msg Message, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Fail != nil {
		return "", r.Fail
	}
	r.Sent = append(r.Sent, msg)
	return "rec-" + key, nil
}

// Messages returns a copy of everything sent so far.
func (r *Recorder) Messages() []Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Message(nil), r.Sent...)
}
