package journal

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Config is the subset of configuration this package needs. The bot's
// top-level config exposes it as DigikeeperLogConfig in cmd/bot/configure.go,
// populated via the DIGIKEEPER_LOG_* env vars.
type Config struct {
	BaseURL    string
	Timeout    time.Duration
	MaxRetries int
	ClientID   string
	Token      string
}

// Client is a synchronous client for the digikeeper-log event-journal
// service. It holds a cached SchemaRepository populated at construction time.
type Client struct {
	cfg        Config
	httpClient *http.Client
	logger     *slog.Logger
	nowFunc    func() time.Time
	overrideID string

	schemas *inMemSchemaRepository
}

// New constructs a Client. It performs two startup probes against the service:
//   - GET /healthz for liveness
//   - GET /v1/registry to populate the local schema cache
//
// Both probes are advisory: on failure we record a warning via the client's
// logger and continue, so the bot can start before the journal service is
// reachable. The schema cache will be empty in that case, and callers can
// call Client.Schemas().Refresh(ctx) later.
func New(ctx context.Context, cfg Config, opts ...Option) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, ErrJournal{Reason: "BaseURL is required", Cause: ErrValidation}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}

	o := options{
		httpClient: &http.Client{Timeout: cfg.Timeout},
		nowFunc:    func() time.Time { return time.Now().UTC() },
		logger:     slog.Default(),
	}
	for _, opt := range opts {
		opt(&o)
	}
	if o.httpClient.Timeout == 0 {
		o.httpClient.Timeout = cfg.Timeout
	}

	c := &Client{
		cfg:        cfg,
		httpClient: o.httpClient,
		logger:     o.logger,
		nowFunc:    o.nowFunc,
		overrideID: o.clientID,
	}
	c.schemas = newInMemSchemaRepository(c)

	c.preflight(ctx)
	return c, nil
}

// preflight runs best-effort startup probes. Failures are logged but do not
// prevent the client from being returned — the journal service may start
// after the bot.
func (c *Client) preflight(ctx context.Context) {
	if err := c.Healthz(ctx); err != nil {
		c.logger.WarnContext(ctx, "journal healthz probe failed", "error", err)
	}
	if err := c.schemas.Refresh(ctx); err != nil {
		c.logger.WarnContext(ctx, "journal schema preflight failed", "error", err)
	}
}

// Append persists one journal event synchronously. It validates the event
// client-side, retries transient failures with jittered exponential backoff
// up to Config.MaxRetries, and honors the caller's context for cancellation
// throughout.
func (c *Client) Append(ctx context.Context, evt Event) (*AppendResult, error) {
	if err := evt.Validate(); err != nil {
		return nil, err
	}
	return c.doAppend(ctx, evt)
}

// Healthz probes GET /healthz. Returns nil if the service responded 2xx.
func (c *Client) Healthz(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/healthz", http.NoBody)
	if err != nil {
		return ErrJournal{Reason: "build healthz request", Cause: err}
	}
	c.setRequestHeaders(req, "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ErrJournal{Reason: "healthz request failed", Cause: fmt.Errorf("%w: %w", ErrTransport, err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrJournal{Reason: fmt.Sprintf("healthz status %d", resp.StatusCode), Cause: ErrServer}
	}
	return nil
}

// Schemas returns the cached SchemaRepository. It is never nil; a client
// whose startup fetch failed will return an empty repository until
// Schemas().Refresh succeeds.
func (c *Client) Schemas() SchemaRepository { return c.schemas }

// Close releases any resources held by the client. Currently a no-op since
// the client holds no background goroutines, but present so callers can
// `defer client.Close()` and not rework if that changes.
func (c *Client) Close() error { return nil }

// clientID returns the effective X-Client-Id — option override takes
// precedence over Config.ClientID.
func (c *Client) clientID() string {
	if c.overrideID != "" {
		return c.overrideID
	}
	return c.cfg.ClientID
}
