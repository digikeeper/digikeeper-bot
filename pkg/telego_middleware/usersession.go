package telegomiddleware

import (
	"context"
	"log/slog"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"

	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
)

type userSessionContextKey struct{}

// SessionKeyFromMessage derives the session key from a message, scoping state
// per chat, per user, and per forum topic/subchat.
//
// ThreadID is left empty (0) unless the message actually belongs to a forum
// topic: MessageThreadID can also be set for reply threads in supergroups, and
// keying on those would fragment a user's session unexpectedly. This mirrors
// aiogram's USER_IN_TOPIC strategy.
func SessionKeyFromMessage(msg *telego.Message) session.SessionKey {
	var threadID int64
	if msg.IsTopicMessage {
		threadID = int64(msg.MessageThreadID)
	}
	return session.SessionKey{
		ChatID:   msg.Chat.ID,
		UserID:   msg.From.ID,
		ThreadID: threadID,
	}
}

type UserSessionMiddleware[S session.UserSession] struct {
	repo session.UserSessionManager[S]
}

func NewUserSessionMiddleware[S session.UserSession](repo session.UserSessionManager[S]) *UserSessionMiddleware[S] {
	return &UserSessionMiddleware[S]{repo: repo}
}

func (um *UserSessionMiddleware[S]) WithUserState(ctx context.Context, state S) context.Context {
	return context.WithValue(ctx, userSessionContextKey{}, state)
}

func (um *UserSessionMiddleware[S]) Middleware() th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		key := SessionKeyFromMessage(update.Message)
		state, err := um.repo.GetOrCreate(ctx, key)
		if err != nil {
			slog.ErrorContext(ctx, "failed to load user session", "error", err)
			return err
		}

		innerCtx := um.WithUserState(ctx.Context(), state)

		return ctx.WithContext(innerCtx).Next(update)
	}
}
