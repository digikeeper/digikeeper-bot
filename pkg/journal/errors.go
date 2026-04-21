package journal

import (
	"errors"
	"fmt"
)

// Sentinel errors that callers can branch on with errors.Is.
var (
	// ErrValidation indicates the supplied Event failed client-side validation.
	ErrValidation = errors.New("journal: validation failed")
	// ErrTransport indicates a network or HTTP-level failure reaching the service.
	ErrTransport = errors.New("journal: transport failure")
	// ErrServer indicates the service responded with a non-2xx status.
	ErrServer = errors.New("journal: server error")
)

// ErrJournal is a wrapper that carries a human-readable reason alongside a
// sentinel cause. It mirrors the style of sessionmanager.ErrSessionManagement
// so callers across the bot see consistent error types.
type ErrJournal struct {
	Reason string
	Cause  error
}

func (e ErrJournal) Error() string {
	if e.Cause == nil {
		return fmt.Sprintf("journal: %s", e.Reason)
	}
	return fmt.Sprintf("journal: %s: %v", e.Reason, e.Cause)
}

func (e ErrJournal) Unwrap() error { return e.Cause }
