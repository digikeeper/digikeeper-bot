package cmdhandler

import (
	"log/slog"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	tu "github.com/mymmrac/telego/telegoutil"
)

var startKeyboard = tu.Keyboard(
	tu.KeyboardRow(
		tu.KeyboardButton("/add"),
		tu.KeyboardButton("/search"),
	),
	tu.KeyboardRow(
		tu.KeyboardButton("More..."),
	),
).WithResizeKeyboard().WithInputFieldPlaceholder("Select action")

func HandleStart(ctx *th.Context, update telego.Update) error {
	slog.InfoContext(ctx.Context(), "Receive /start")
	chatId := tu.ID(update.Message.Chat.ID)
	_, err := ctx.Bot().SendMessage(ctx, tu.Message(
		chatId,
		"Hello! I'm a digikeeper bot. I can help you to keep your digital twin in order.",
	).WithReplyMarkup(startKeyboard))
	if err != nil {
		return err
	}

	return nil
}
