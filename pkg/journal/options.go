package journal

import (
	"log/slog"
	"net/http"
	"time"
)

// Option customizes a Client at construction time.
type Option func(*options)

type options struct {
	httpClient *http.Client
	nowFunc    func() time.Time
	logger     *slog.Logger
	clientID   string
}

// WithHTTPClient overrides the underlying *http.Client. Useful in tests
// (httptest.NewServer().Client()) and when a caller needs custom transport
// settings (proxies, timeouts beyond the request level, etc.).
func WithHTTPClient(hc *http.Client) Option {
	return func(o *options) { o.httpClient = hc }
}

// WithNowFunc overrides the clock used by typed event constructors that call
// into the client. Primarily for deterministic tests.
func WithNowFunc(nf func() time.Time) Option {
	return func(o *options) { o.nowFunc = nf }
}

// WithLogger overrides the slog.Logger the client uses for its own
// diagnostics. Defaults to slog.Default().
func WithLogger(l *slog.Logger) Option {
	return func(o *options) { o.logger = l }
}

// WithClientID overrides the X-Client-Id header value. Defaults to the
// ClientID in the Config; WithClientID wins if both are set.
func WithClientID(id string) Option {
	return func(o *options) { o.clientID = id }
}
