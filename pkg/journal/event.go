package journal

import "time"

// Event is the wire shape accepted by POST /v1/logs on the digikeeper-log
// service. Field names and types match
// internal/httpapi/command/handler.go:AppendInput.Body on the server.
type Event struct {
	Type      string         `json:"type,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Tags      []string       `json:"tags"`
	Data      map[string]any `json:"data"`
}

// Validate mirrors the server-side Resolve() check so bad events fail without
// a round-trip. The server requires a non-zero timestamp AND at least one of
// tags or data to be present.
func (e Event) Validate() error {
	if e.Timestamp.IsZero() {
		return ErrJournal{Reason: "timestamp is required", Cause: ErrValidation}
	}
	if len(e.Tags) == 0 && len(e.Data) == 0 {
		return ErrJournal{Reason: "at least one of tags or data must be provided", Cause: ErrValidation}
	}
	return nil
}

// AppendResult describes a successfully persisted journal entry.
type AppendResult struct {
	ID        string
	Indexed   bool
	RequestID string
}

// TypeNoteAdded is the provisional type string for "a note was added via the
// bot." The digikeeper-log service does not publish a canonical type
// taxonomy; when it does, this constant is the single point of change.
const TypeNoteAdded = "note.added"

// NewNoteAddedEvent builds a journal Event for a note that the user added
// through the bot. It stamps the current time via time.Now; callers that
// want a deterministic clock should construct the Event directly or use
// WithNowFunc on the client.
func NewNoteAddedEvent(userID int64, note string) Event {
	return Event{
		Type:      TypeNoteAdded,
		Timestamp: time.Now().UTC(),
		Tags:      []string{"bot", "note"},
		Data: map[string]any{
			"user_id": userID,
			"note":    note,
		},
	}
}
