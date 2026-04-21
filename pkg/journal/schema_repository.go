package journal

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
)

// SchemaEntry is one schema published by the service under GET /v1/registry.
// The Schema field is the raw JSON-Schema document.
type SchemaEntry struct {
	Type   string          `json:"type"`
	Schema json.RawMessage `json:"schema"`
}

// SchemaRepository is the read-only view callers use to look up the entry
// schemas the service publishes. The repository is populated from the server
// at startup and cached in memory for the process lifetime; Refresh re-pulls
// from the server on demand.
type SchemaRepository interface {
	Get(entryType string) (SchemaEntry, bool)
	All() []SchemaEntry
	Refresh(ctx context.Context) error
}

// inMemSchemaRepository is the default SchemaRepository: a goroutine-safe
// map backed by a single upstream fetch.
type inMemSchemaRepository struct {
	fetcher schemaFetcher

	mu      sync.RWMutex
	byType  map[string]SchemaEntry
	ordered []string
}

type schemaFetcher interface {
	fetchSchemas(ctx context.Context) ([]SchemaEntry, error)
}

func newInMemSchemaRepository(f schemaFetcher) *inMemSchemaRepository {
	return &inMemSchemaRepository{
		fetcher: f,
		byType:  make(map[string]SchemaEntry),
	}
}

func (r *inMemSchemaRepository) Get(entryType string) (SchemaEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.byType[entryType]
	return s, ok
}

func (r *inMemSchemaRepository) All() []SchemaEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]SchemaEntry, 0, len(r.ordered))
	for _, t := range r.ordered {
		out = append(out, r.byType[t])
	}
	return out
}

func (r *inMemSchemaRepository) Refresh(ctx context.Context) error {
	entries, err := r.fetcher.fetchSchemas(ctx)
	if err != nil {
		return err
	}

	byType := make(map[string]SchemaEntry, len(entries))
	ordered := make([]string, 0, len(entries))
	for _, e := range entries {
		byType[e.Type] = e
		ordered = append(ordered, e.Type)
	}

	r.mu.Lock()
	r.byType = byType
	r.ordered = ordered
	r.mu.Unlock()
	return nil
}

// schemaListResponse matches the body returned by GET /v1/registry:
//
//	{"schemas": [{"type": "...", "schema": {...}}, ...]}
type schemaListResponse struct {
	Schemas []SchemaEntry `json:"schemas"`
}

// fetchSchemas is the concrete HTTP fetch used by Client to populate the
// repository. It lives on the Client rather than the repository so the
// repository stays free of HTTP plumbing.
func (c *Client) fetchSchemas(ctx context.Context) ([]SchemaEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+"/v1/registry", http.NoBody)
	if err != nil {
		return nil, ErrJournal{Reason: "build registry request", Cause: err}
	}
	c.setRequestHeaders(req, "")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrJournal{Reason: "registry request failed", Cause: fmt.Errorf("%w: %w", ErrTransport, err)}
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, ErrJournal{Reason: fmt.Sprintf("registry status %d", resp.StatusCode), Cause: ErrServer}
	}

	var body schemaListResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, ErrJournal{Reason: "decode registry response", Cause: err}
	}
	return body.Schemas, nil
}
