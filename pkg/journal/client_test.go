package journal_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/gitrus/digikeeper-bot/pkg/journal"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeString(t *testing.T, w http.ResponseWriter, s string) {
	t.Helper()
	if _, err := io.WriteString(w, s); err != nil {
		t.Fatalf("write response: %v", err)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	if err := json.NewEncoder(w).Encode(v); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

type serverHandlers struct {
	append   http.HandlerFunc
	registry http.HandlerFunc
	healthz  http.HandlerFunc
}

func newServer(t *testing.T, h serverHandlers) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	if h.append != nil {
		mux.HandleFunc("POST /v1/logs", h.append)
	}
	if h.registry != nil {
		mux.HandleFunc("GET /v1/registry", h.registry)
	}
	if h.healthz != nil {
		mux.HandleFunc("GET /healthz", h.healthz)
	} else {
		mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func defaultRegistry(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		writeString(t, w, `{"schemas":[{"type":"entry","schema":{"type":"object"}}]}`)
	}
}

func newClient(t *testing.T, srv *httptest.Server) *journal.Client {
	t.Helper()
	cfg := journal.Config{
		BaseURL:    srv.URL,
		Timeout:    2 * time.Second,
		MaxRetries: 2,
		ClientID:   "digikeeper-bot-test",
	}
	cli, err := journal.New(t.Context(), cfg)
	assert.NoError(t, err)
	return cli
}

func appendOK(t *testing.T, status int, id, reqID string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.api+json")
		w.WriteHeader(status)
		writeJSON(t, w, map[string]any{
			"meta": map[string]any{"type": "logs"},
			"data": map[string]any{
				"id": id,
				"attributes": map[string]any{
					"request_id": reqID,
				},
			},
		})
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

func TestEvent_Validate(t *testing.T) {
	cases := []struct {
		name   string
		evt    journal.Event
		expect error
	}{
		{
			name:   "zero timestamp rejected",
			evt:    journal.Event{Tags: []string{"bot"}},
			expect: journal.ErrValidation,
		},
		{
			name:   "empty tags and data rejected",
			evt:    journal.Event{Timestamp: time.Now()},
			expect: journal.ErrValidation,
		},
		{
			name: "ok with tags only",
			evt:  journal.Event{Timestamp: time.Now(), Tags: []string{"bot"}},
		},
		{
			name: "ok with data only",
			evt:  journal.Event{Timestamp: time.Now(), Data: map[string]any{"k": "v"}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.evt.Validate()
			if tc.expect == nil {
				assert.NoError(t, err)
				return
			}
			assert.Error(t, err)
			assert.ErrorIs(t, err, tc.expect)
		})
	}
}

func TestNewNoteAddedEvent(t *testing.T) {
	evt := journal.NewNoteAddedEvent(42, "buy milk")
	assert.Equal(t, journal.TypeNoteAdded, evt.Type)
	assert.Equal(t, []string{"bot", "note"}, evt.Tags)
	assert.Equal(t, int64(42), evt.Data["user_id"])
	assert.Equal(t, "buy milk", evt.Data["note"])
	assert.NoError(t, evt.Validate())
}

// ---------------------------------------------------------------------------
// Append: happy paths
// ---------------------------------------------------------------------------

func TestClient_Append_201(t *testing.T) {
	var captured struct {
		clientID string
		reqID    string
		body     journal.Event
	}
	srv := newServer(t, serverHandlers{
		append: func(w http.ResponseWriter, r *http.Request) {
			captured.clientID = r.Header.Get("X-Client-Id")
			captured.reqID = r.Header.Get("X-Request-ID")
			if err := json.NewDecoder(r.Body).Decode(&captured.body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			appendOK(t, http.StatusCreated, "entry-1", captured.reqID)(w, r)
		},
		registry: defaultRegistry(t),
	})

	cli := newClient(t, srv)
	defer func() { _ = cli.Close() }()

	res, err := cli.Append(t.Context(), journal.NewNoteAddedEvent(1, "note"))
	assert.NoError(t, err)
	assert.Equal(t, "entry-1", res.ID)
	assert.True(t, res.Indexed)
	assert.NotEmpty(t, res.RequestID)
	assert.Equal(t, "digikeeper-bot-test", captured.clientID)
	assert.NotEmpty(t, captured.reqID)
	assert.Equal(t, journal.TypeNoteAdded, captured.body.Type)
}

func TestClient_Append_202_IndexedFalse(t *testing.T) {
	srv := newServer(t, serverHandlers{
		append:   appendOK(t, http.StatusAccepted, "e-2", "rid"),
		registry: defaultRegistry(t),
	})
	cli := newClient(t, srv)

	res, err := cli.Append(t.Context(), journal.NewNoteAddedEvent(1, "n"))
	assert.NoError(t, err)
	assert.Equal(t, "e-2", res.ID)
	assert.False(t, res.Indexed)
}

// ---------------------------------------------------------------------------
// Append: retries, cancellation, auth
// ---------------------------------------------------------------------------

func TestClient_Append_RetriesOn5xxThenSucceeds(t *testing.T) {
	var calls int32
	srv := newServer(t, serverHandlers{
		append: func(w http.ResponseWriter, r *http.Request) {
			n := atomic.AddInt32(&calls, 1)
			if n < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			appendOK(t, http.StatusCreated, "ok-"+r.Header.Get("X-Request-ID"), "")(w, r)
		},
		registry: defaultRegistry(t),
	})
	cli := newClient(t, srv)

	res, err := cli.Append(t.Context(), journal.NewNoteAddedEvent(1, "n"))
	assert.NoError(t, err)
	assert.NotEmpty(t, res.ID)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls))
}

func TestClient_Append_5xxExhaustsRetries(t *testing.T) {
	var calls int32
	srv := newServer(t, serverHandlers{
		append: func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusServiceUnavailable)
		},
		registry: defaultRegistry(t),
	})
	cli := newClient(t, srv)

	_, err := cli.Append(t.Context(), journal.NewNoteAddedEvent(1, "n"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, journal.ErrServer)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls)) // MaxRetries=2 → 3 attempts
}

func TestClient_Append_4xxNotRetried(t *testing.T) {
	var calls int32
	srv := newServer(t, serverHandlers{
		append: func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusBadRequest)
		},
		registry: defaultRegistry(t),
	})
	cli := newClient(t, srv)

	_, err := cli.Append(t.Context(), journal.NewNoteAddedEvent(1, "n"))
	assert.Error(t, err)
	assert.ErrorIs(t, err, journal.ErrServer)
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls))
}

func TestClient_Append_ContextCancelAbortsRetryLoop(t *testing.T) {
	srv := newServer(t, serverHandlers{
		append: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
		registry: defaultRegistry(t),
	})
	cli := newClient(t, srv)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := cli.Append(ctx, journal.NewNoteAddedEvent(1, "n"))
	assert.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled), "expected ctx.Canceled, got %v", err)
}

func TestClient_Append_InvalidEventNotSent(t *testing.T) {
	var calls int32
	srv := newServer(t, serverHandlers{
		append: func(w http.ResponseWriter, _ *http.Request) {
			atomic.AddInt32(&calls, 1)
			w.WriteHeader(http.StatusCreated)
		},
		registry: defaultRegistry(t),
	})
	cli := newClient(t, srv)

	_, err := cli.Append(t.Context(), journal.Event{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, journal.ErrValidation)
	assert.Equal(t, int32(0), atomic.LoadInt32(&calls))
}

func TestClient_Append_AuthHeaderPropagation(t *testing.T) {
	var got string
	srv := newServer(t, serverHandlers{
		append: func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("Authorization")
			appendOK(t, http.StatusCreated, "id", r.Header.Get("X-Request-ID"))(w, r)
		},
		registry: defaultRegistry(t),
	})

	cfg := journal.Config{
		BaseURL:    srv.URL,
		Timeout:    2 * time.Second,
		MaxRetries: 0,
		ClientID:   "bot",
		Token:      "s3cret",
	}
	cli, err := journal.New(t.Context(), cfg)
	assert.NoError(t, err)

	_, err = cli.Append(t.Context(), journal.NewNoteAddedEvent(1, "n"))
	assert.NoError(t, err)
	assert.Equal(t, "Bearer s3cret", got)
}

func TestClient_Append_NoAuthHeaderWhenTokenEmpty(t *testing.T) {
	var got string
	srv := newServer(t, serverHandlers{
		append: func(w http.ResponseWriter, r *http.Request) {
			got = r.Header.Get("Authorization")
			appendOK(t, http.StatusCreated, "id", r.Header.Get("X-Request-ID"))(w, r)
		},
		registry: defaultRegistry(t),
	})
	cli := newClient(t, srv) // no Token set

	_, err := cli.Append(t.Context(), journal.NewNoteAddedEvent(1, "n"))
	assert.NoError(t, err)
	assert.Empty(t, got)
}

// ---------------------------------------------------------------------------
// Schema repository
// ---------------------------------------------------------------------------

func TestClient_SchemaPreflight_CachesCatalog(t *testing.T) {
	srv := newServer(t, serverHandlers{
		registry: func(w http.ResponseWriter, _ *http.Request) {
			writeString(t, w, `{"schemas":[
				{"type":"entry","schema":{"k":"v"}},
				{"type":"other","schema":{"x":1}}
			]}`)
		},
	})
	cli := newClient(t, srv)

	repo := cli.Schemas()
	assert.NotNil(t, repo)

	entry, ok := repo.Get("entry")
	assert.True(t, ok)
	assert.Equal(t, "entry", entry.Type)
	assert.JSONEq(t, `{"k":"v"}`, string(entry.Schema))

	_, ok = repo.Get("missing")
	assert.False(t, ok)

	all := repo.All()
	assert.Len(t, all, 2)
}

func TestClient_SchemaPreflight_FailsSoft(t *testing.T) {
	srv := newServer(t, serverHandlers{
		registry: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		},
	})
	// No error: preflight is advisory.
	cli := newClient(t, srv)
	assert.Empty(t, cli.Schemas().All())

	// A later Refresh succeeds once the server comes back.
	srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/registry" {
			writeString(t, w, `{"schemas":[{"type":"entry","schema":{"z":true}}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	err := cli.Schemas().Refresh(t.Context())
	assert.NoError(t, err)
	entry, ok := cli.Schemas().Get("entry")
	assert.True(t, ok)
	assert.JSONEq(t, `{"z":true}`, string(entry.Schema))
}

// ---------------------------------------------------------------------------
// New / config validation
// ---------------------------------------------------------------------------

func TestNew_BaseURLRequired(t *testing.T) {
	_, err := journal.New(t.Context(), journal.Config{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, journal.ErrValidation)
}
