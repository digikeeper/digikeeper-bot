package telegocommandrouter

import (
	"log/slog"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

const DefaultUnknownCommandMessage = "Unknown command. Type /help to see available commands."

func NewUnknownCommandHandler(unknownCommandMsg string) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		cmd := ""
		if update.Message != nil && update.Message.Text != "" {
			cmd = update.Message.Text
		}
		slog.InfoContext(
			ctx.Context(),
			"Unknown command",
			slog.String("command", cmd),
		)

		chatID := tu.ID(update.Message.Chat.ID)
		_, err := ctx.Bot().SendMessage(ctx, tu.Message(
			chatID,
			unknownCommandMsg,
		))
		if err != nil {
			slog.ErrorContext(ctx.Context(), "Failed to send unknown command message", slog.Any("error", err))
		}

		return err
	}
}
