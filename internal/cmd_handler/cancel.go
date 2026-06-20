package cmdhandler

import (
	"log/slog"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
)

type CancelHandler struct {
	usm session.UserSessionManager[*session.SimpleUserSession]
}

func NewCancelHandler(
	usm session.UserSessionManager[*session.SimpleUserSession],
) *CancelHandler {
	return &CancelHandler{usm: usm}
}

func (ch *CancelHandler) Handle(ctx *th.Context, update telego.Update) error {
	slog.InfoContext(ctx.Context(), "Receive /cancel")

	userID := update.Message.From.ID
	ch.usm.DropActive(ctx, userID)

	chatId := tu.ID(update.Message.Chat.ID)
	_, err := ctx.Bot().SendMessage(ctx, tu.Message(
		chatId,
		"I just interrupted the current operation/s. What can I do for you now?",
	))
	if err != nil {
		slog.ErrorContext(ctx.Context(), "Failed to send message")
		return err
	}

	return nil
}
