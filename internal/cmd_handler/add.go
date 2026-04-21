package cmdhandler

import (
	"log/slog"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	"github.com/gitrus/digikeeper-bot/pkg/journal"
	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
)

// stateAwaitingNote marks a user session that entered /add and is now
// expected to send the note contents as the next plain-text message.
const stateAwaitingNote = "add:awaiting_note"

// HandleAdd starts the /add flow: it transitions the user's session to
// "awaiting note" and prompts them for the note contents. The actual note
// persistence happens in HandleAddNoteText when the user replies.
func HandleAdd(usm session.UserSessionManager[*session.SimpleUserSession]) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		slog.InfoContext(ctx.Context(), "Receive /add")

		userID := update.Message.From.ID
		state, err := usm.Fetch(ctx, userID)
		if err != nil {
			return err
		}

		_, err = usm.Set(
			ctx,
			userID,
			&session.SimpleUserSession{UserID: userID, State: stateAwaitingNote, Version: state.Version + 1},
			state.Version,
		)
		if err != nil {
			slog.ErrorContext(ctx.Context(), "Failed to set state", "error", err)

			chatId := tu.ID(update.Message.Chat.ID)
			_, err = ctx.Bot().SendMessage(ctx, tu.Message(
				chatId,
				"Another action is in progress. Please finish it first.",
			))
			return err
		}

		chatID := tu.ID(update.Message.Chat.ID)
		_, err = ctx.Bot().SendMessage(ctx, tu.Message(
			chatID,
			"What should I note down? Send the text of the note, or /cancel to abort.",
		))
		return err
	}
}

// HandleAddNoteText catches the next plain-text message from a user whose
// session is in stateAwaitingNote, persists it as a note.added journal event
// on digikeeper-log, and clears the state. Users not in the awaiting-note
// state are passed through via ctx.Next so other handlers can match.
//
// journal may be nil when the journal client is disabled in config; in that
// case the text is ignored with a user-visible notice instead of being lost.
func HandleAddNoteText(
	usm session.UserSessionManager[*session.SimpleUserSession],
	events *journal.Client,
) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		if update.Message == nil || update.Message.From == nil {
			return ctx.Next(update)
		}
		userID := update.Message.From.ID

		state, err := usm.Fetch(ctx, userID)
		if err != nil || state.State != stateAwaitingNote {
			return ctx.Next(update)
		}

		chatID := tu.ID(update.Message.Chat.ID)

		if events == nil {
			slog.WarnContext(ctx.Context(), "journal client disabled, note dropped")
			_, sendErr := ctx.Bot().SendMessage(ctx, tu.Message(
				chatID,
				"Note storage isn't configured right now — please try again later.",
			))
			if dropErr := usm.DropActive(ctx, userID); dropErr != nil {
				slog.WarnContext(ctx.Context(), "Failed to drop session", "error", dropErr)
			}
			return sendErr
		}

		note := update.Message.Text
		if note == "" {
			_, err = ctx.Bot().SendMessage(ctx, tu.Message(
				chatID,
				"Empty note — please send the note text, or /cancel to abort.",
			))
			return err
		}

		res, err := events.Append(ctx.Context(), journal.NewNoteAddedEvent(userID, note))
		if err != nil {
			slog.ErrorContext(ctx.Context(), "Failed to append note event", "error", err)
			_, sendErr := ctx.Bot().SendMessage(ctx, tu.Message(
				chatID,
				"I couldn't save the note right now. Please try again.",
			))
			if sendErr != nil {
				return sendErr
			}
			return err
		}

		slog.InfoContext(ctx.Context(), "Note added",
			slog.String("entry_id", res.ID),
			slog.String("request_id", res.RequestID),
			slog.Bool("indexed", res.Indexed),
		)

		if dropErr := usm.DropActive(ctx, userID); dropErr != nil {
			slog.WarnContext(ctx.Context(), "Failed to drop session after note add", "error", dropErr)
		}

		_, err = ctx.Bot().SendMessage(ctx, tu.Message(
			chatID,
			"Noted. ✔",
		))
		return err
	}
}
