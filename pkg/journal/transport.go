package journal

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	contentType    = "application/json"
	headerClientID = "X-Client-Id"
	headerReqID    = "X-Request-ID"
	headerAuth     = "Authorization"
)

// appendResponse mirrors the server's envelope from
// internal/httpapi/resource.go: {meta: {...}, data: {id, attributes: {...}}}.
// Only the fields the client actually returns to callers are deserialized.
type appendResponse struct {
	Data struct {
		ID         string `json:"id"`
		Attributes struct {
			RequestID string `json:"request_id"`
		} `json:"attributes"`
	} `json:"data"`
}

// doAppend sends one POST /v1/logs attempt with retries.
func (c *Client) doAppend(ctx context.Context, evt Event) (*AppendResult, error) {
	body, err := json.Marshal(evt)
	if err != nil {
		return nil, ErrJournal{Reason: "marshal event", Cause: err}
	}

	requestID, err := newRequestID()
	if err != nil {
		return nil, ErrJournal{Reason: "generate request id", Cause: err}
	}

	var lastErr error
	for attempt := 0; attempt <= c.cfg.MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, ErrJournal{Reason: "context cancelled", Cause: err}
		}

		result, retryable, err := c.appendOnce(ctx, body, requestID)
		if err == nil {
			return result, nil
		}
		lastErr = err

		if !retryable || attempt == c.cfg.MaxRetries {
			return nil, lastErr
		}
		if err := sleepWithCtx(ctx, backoff(attempt)); err != nil {
			return nil, ErrJournal{Reason: "context cancelled during backoff", Cause: err}
		}
	}
	return nil, lastErr
}

// appendOnce performs a single HTTP round-trip. The returned bool reports
// whether the error (if any) is retryable.
func (c *Client) appendOnce(ctx context.Context, body []byte, requestID string) (*AppendResult, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/v1/logs", bytes.NewReader(body))
	if err != nil {
		return nil, false, ErrJournal{Reason: "build append request", Cause: err}
	}
	req.Header.Set("Content-Type", contentType)
	c.setRequestHeaders(req, requestID)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, true, ErrJournal{Reason: "append request failed", Cause: fmt.Errorf("%w: %w", ErrTransport, err)}
	}
	defer func() { _ = resp.Body.Close() }()

	switch resp.StatusCode {
	case http.StatusCreated, http.StatusAccepted:
		result, err := parseAppendResponse(resp, requestID)
		return result, false, err
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusUnprocessableEntity:
		return nil, false, readErrorResponse(resp)
	default:
		if resp.StatusCode >= 500 {
			return nil, true, readErrorResponse(resp)
		}
		return nil, false, readErrorResponse(resp)
	}
}

func parseAppendResponse(resp *http.Response, requestID string) (*AppendResult, error) {
	var body appendResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, ErrJournal{Reason: "decode append response", Cause: err}
	}
	returnedReqID := body.Data.Attributes.RequestID
	if returnedReqID == "" {
		returnedReqID = requestID
	}
	return &AppendResult{
		ID:        body.Data.ID,
		Indexed:   resp.StatusCode == http.StatusCreated,
		RequestID: returnedReqID,
	}, nil
}

func readErrorResponse(resp *http.Response) error {
	snippet, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
	reason := fmt.Sprintf("status %d", resp.StatusCode)
	if err == nil && len(snippet) > 0 {
		reason = fmt.Sprintf("%s: %s", reason, bytes.TrimSpace(snippet))
	}
	return ErrJournal{Reason: reason, Cause: ErrServer}
}

func (c *Client) setRequestHeaders(req *http.Request, requestID string) {
	clientID := c.clientID()
	if clientID != "" {
		req.Header.Set(headerClientID, clientID)
	}
	if requestID != "" {
		req.Header.Set(headerReqID, requestID)
	}
	if c.cfg.Token != "" {
		req.Header.Set(headerAuth, "Bearer "+c.cfg.Token)
	}
	req.Header.Set("Accept", "application/vnd.api+json, application/json")
}

func newRequestID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func backoff(attempt int) time.Duration {
	// 100ms, 200ms, 400ms, 800ms ... capped at 2s.
	base := 100 * time.Millisecond
	d := base << attempt
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	// Cheap jitter: ± 25%. If rand fails we fall back to the un-jittered value.
	jitter := int64(d / 4)
	var j [1]byte
	if _, err := rand.Read(j[:]); err != nil {
		return d
	}
	off := (int64(j[0]) - 128) * jitter / 128
	return d + time.Duration(off)
}

func sleepWithCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
