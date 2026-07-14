package sessionmanager

import (
	"context"
	"fmt"
	"sync"
)

// UserSessionManagerInMem is an in-memory UserSessionManager backed by sync.Map.
// It provides thread-safe operations for managing user sessions in memory.
type UserSessionManagerInMem[S UserSession] struct {
	sessions sync.Map // key: SessionKey, value: S

	userSessionFabric NewUserSession[S]
}

type ErrSessionManagement struct {
	Reason string
}

func (e ErrSessionManagement) Error() string {
	return fmt.Sprintf("session fetch error %s", e.Reason)
}

func NewUserSessionManagerInMem[S UserSession](usf NewUserSession[S]) *UserSessionManagerInMem[S] {
	return &UserSessionManagerInMem[S]{userSessionFabric: usf}
}

// GetOrCreate returns the existing session for key or atomically stores a new
// one from NewUserSession. It never replaces an existing session.
func (usm *UserSessionManagerInMem[S]) GetOrCreate(_ context.Context, key SessionKey) (S, error) {
	var zero S
	if value, ok := usm.sessions.Load(key); ok {
		state, ok := value.(S)
		if !ok {
			return zero, ErrSessionManagement{
				Reason: fmt.Sprintf("user session type assertion failed for %s", key),
			}
		}
		return state, nil
	}

	newSession, err := usm.userSessionFabric(key)
	if err != nil {
		return zero, err
	}
	value, loaded := usm.sessions.LoadOrStore(key, newSession)
	if !loaded {
		return newSession, nil
	}

	state, ok := value.(S)
	if !ok {
		return zero, ErrSessionManagement{
			Reason: fmt.Sprintf("user session type assertion failed for %s", key),
		}
	}
	return state, nil
}

// InitSession is retained for backwards compatibility. New callers should use
// GetOrCreate, which does not replace an existing session.
func (usm *UserSessionManagerInMem[S]) InitSession(ctx context.Context, key SessionKey) (S, error) {
	return usm.GetOrCreate(ctx, key)
}

func (m *UserSessionManagerInMem[S]) Fetch(
	_ context.Context, key SessionKey,
) (S, error) {
	var result S
	if value, ok := m.sessions.Load(key); ok {
		if state, ok := value.(S); ok {
			return state, nil
		}
		return result, ErrSessionManagement{
			Reason: fmt.Sprintf("user session type assertion failed for %s", key),
		}
	}

	return result, fmt.Errorf("%w for %s", ErrSessionNotFound, key)
}

// DropActive removes the session for the specified key.
func (m *UserSessionManagerInMem[S]) DropActive(_ context.Context, key SessionKey) error {
	m.sessions.Delete(key)
	return nil
}

// Set updates the session for the specified key and version.
// Returns the new session if successful, or an error.
func (m *UserSessionManagerInMem[S]) Set(
	_ context.Context, key SessionKey, newSession S, prevVersion int,
) (S, error) {
	oldValue, loaded := m.sessions.Load(key)
	if !loaded {
		return newSession, ErrSessionManagement{Reason: "session not found"}
	}
	oldSession, ok := oldValue.(S)
	if !ok {
		var zero S
		return zero, ErrSessionManagement{
			Reason: fmt.Sprintf("user session type assertion failed for %s", key),
		}
	}
	if oldSession.GetVersion() != prevVersion {
		return oldSession, ErrSessionManagement{Reason: "version mismatch"}
	}

	swapped := m.sessions.CompareAndSwap(key, oldSession, newSession)
	if !swapped {
		err := ErrSessionManagement{
			Reason: fmt.Sprintf("failed to Set for %s", key),
		}
		return oldSession, err
	}

	return newSession, nil
}

// SimpleUserSession is a minimal session carrying its identifying key plus a
// single FSM state string.
type SimpleUserSession struct {
	SessionKey

	State   string
	Version int
}

func NewSimpleUserSession(key SessionKey) (*SimpleUserSession, error) {
	return &SimpleUserSession{
		SessionKey: key,
		State:      "",
		Version:    1,
	}, nil
}

func (s SimpleUserSession) GetVersion() int {
	return s.Version
}
