package sessionmanager

import (
	"context"
	"errors"
	"fmt"
)

// SessionKey identifies a single session. Following Telegram bot convention
// (cf. aiogram's USER_IN_TOPIC FSM strategy), state is scoped per user, per
// chat, and per forum topic/subchat: the same user has independent sessions in
// different chats or topics. ThreadID is 0 outside of forum topics.
type SessionKey struct {
	ChatID   int64
	UserID   int64
	ThreadID int64
}

func (k SessionKey) String() string {
	return fmt.Sprintf("user %d in chat %d thread %d", k.UserID, k.ChatID, k.ThreadID)
}

// ErrSessionNotFound indicates that no active session exists for a key.
// Callers may use errors.Is to distinguish this expected condition from
// persistence or decoding failures.
var ErrSessionNotFound = errors.New("session not found")

type UserSession interface {
	GetVersion() int
}

type NewUserSession[S UserSession] func(key SessionKey) (S, error)

type UserSessionManager[S UserSession] interface {
	GetOrCreate(ctx context.Context, key SessionKey) (S, error)
	Fetch(ctx context.Context, key SessionKey) (S, error)
	Set(ctx context.Context, key SessionKey, newSession S, prevVersion int) (S, error)
	DropActive(ctx context.Context, key SessionKey) error
}
