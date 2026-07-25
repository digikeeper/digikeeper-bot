package telegomiddleware

import (
	"log/slog"

	"github.com/gitrus/digikeeper-bot/pkg/loggingctx"
	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// FirstNRunes returns the first n runes of a string.
func FirstNRunes(s string, n int) string {
	runes := []rune(s)
	if n > len(runes) {
		n = len(runes)
	}
	return string(runes[:n])
}

func AddUpdateSlogAttrs() th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		origCtx := ctx.Context()

		innerCtx := loggingctx.AddLogAttr(origCtx, "update_id", update.UpdateID)

		if update.Message == nil {
			ctx = ctx.WithContext(innerCtx)
			return ctx.Next(update)
		}

		// Add more attributes for messages
		innerCtx = loggingctx.AddLogAttrs(innerCtx, []slog.Attr{
			slog.Int("message_id", update.Message.MessageID),
			slog.String("text_first10", FirstNRunes(update.Message.Text, 10)),
			slog.Int64("chat_id", update.Message.Chat.ID),
		})

		if update.Message.From != nil {
			innerCtx = loggingctx.AddLogAttr(innerCtx, "user_id", update.Message.From.ID)
		}

		ctx = ctx.WithContext(innerCtx)

		return ctx.Next(update)
	}
}
