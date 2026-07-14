package cmdhandler

import (
	"log/slog"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"

	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
	tm "github.com/gitrus/digikeeper-bot/pkg/telego_middleware"
)

type AddHandler struct {
	usm session.UserSessionManager[*session.SimpleUserSession]
}

func NewAddHandler(usm session.UserSessionManager[*session.SimpleUserSession]) *AddHandler {
	return &AddHandler{usm: usm}
}

func (ah *AddHandler) Handle(ctx *th.Context, update telego.Update) error {
	slog.InfoContext(ctx.Context(), "Receive /add")

	key := tm.SessionKeyFromMessage(update.Message)
	state, err := ah.usm.Fetch(ctx, key)
	if err != nil {
		return err
	}
	chatId := tu.ID(update.Message.Chat.ID)

	_, err = ah.usm.Set(
		ctx,
		key,
		&session.SimpleUserSession{SessionKey: key, State: "add", Version: state.Version + 1},
		state.Version,
	)
	if err != nil {
		slog.ErrorContext(ctx.Context(), "Failed to set state")

		_, err = ctx.Bot().SendMessage(ctx, tu.Message(
			chatId,
			"Another action is in progress. Please finish it first.",
		))
		return err
	}

	_, err = ctx.Bot().SendMessage(ctx, tu.Message(
		chatId,
		"",
	).WithReplyMarkup(startKeyboard))
	if err != nil {
		return err
	}

	slog.InfoContext(ctx.Context(), "Set state", slog.String("state", state.State))
	return nil
}
