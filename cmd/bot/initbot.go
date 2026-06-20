package main

import (
	"context"
	"log"

	"github.com/mymmrac/telego"
	"github.com/valyala/fasthttp"
)

func initBot(ctx context.Context, cfg Config) (*telego.Bot, <-chan telego.Update, error) {
	bot, err := telego.NewBot(cfg.Telegram.BotKey.String(), telego.WithDefaultDebugLogger())
	if err != nil {
		log.Fatalf("Failed to start bot: %v", err)
	}

	var upd <-chan telego.Update
	if !cfg.IsDevEnv() {
		upd, err = initWebHookBot(ctx, cfg, bot)
	} else {
		upd, err = initPollingBot(ctx, cfg, bot)
	}

	return bot, upd, err
}

func initWebHookBot(ctx context.Context, cfg Config, bot *telego.Bot) (<-chan telego.Update, error) {
	err := bot.SetWebhook(ctx, &telego.SetWebhookParams{
		URL:            cfg.Telegram.PublicURL,
		SecretToken:    bot.Token(),
		AllowedUpdates: cfg.Telegram.AllowedUpdates,
	})
	if err != nil {
		return nil, err
	}

	srv := &fasthttp.Server{}

	updates, err := bot.UpdatesViaWebhook(
		ctx,
		telego.WebhookFastHTTP(
			srv, "/bot", cfg.Telegram.WebHookSecretToken,
		),
	)

	return updates, err
}

func initPollingBot(ctx context.Context, cfg Config, bot *telego.Bot) (<-chan telego.Update, error) {
	updates, err := bot.UpdatesViaLongPolling(
		ctx,
		&telego.GetUpdatesParams{AllowedUpdates: cfg.Telegram.AllowedUpdates},
	)
	return updates, err
}
