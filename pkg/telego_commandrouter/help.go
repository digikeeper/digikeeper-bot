package telegocommandrouter

import (
	"log/slog"
	"strings"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

// NewHelpHandler creates a handler function for the /help command.
// It generates a help message listing available commands based on the provided descriptions.
func NewHelpHandler(cmdDescriptions map[string]string) th.Handler {
	return func(ctx *th.Context, update telego.Update) error {
		slog.InfoContext(ctx.Context(), "Receive /help")

		helpMessageBuilder := strings.Builder{}
		helpMessageBuilder.WriteString("Available commands:\n")
		helpMessageBuilder.WriteString("/help    Show this help message\n")
		for command, description := range cmdDescriptions {
			if command == "help" {
				continue
			}
			helpMessageBuilder.WriteString("/" + command)
			helpMessageBuilder.WriteString("    " + description)
			helpMessageBuilder.WriteString("\n")
		}

		chatID := tu.ID(update.Message.Chat.ID)
		_, err := ctx.Bot().SendMessage(ctx, tu.Message(chatID, helpMessageBuilder.String()))
		if err != nil {
			slog.ErrorContext(ctx.Context(), "Failed to send help message", slog.Any("error", err))
		}

		return err
	}
}
