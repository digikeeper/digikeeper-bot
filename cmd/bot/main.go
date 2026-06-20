package main

import (
	"context"
	"log/slog"

	th "github.com/mymmrac/telego/telegohandler"

	cmdh "github.com/gitrus/digikeeper-bot/internal/cmd_handler"
	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
	cmdrouter "github.com/gitrus/digikeeper-bot/pkg/telego_commandrouter"
	tm "github.com/gitrus/digikeeper-bot/pkg/telego_middleware"
)

func main() {
	config := configure()
	logger := slog.Default()

	ctx := context.Background()

	bot, updates, err := initBot(ctx, config)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to init bot: %v", "error", err)
		return
	}

	bh, err := th.NewBotHandler(bot, updates)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to add handler bot", "error", err)
		return
	}
	defer func() { _ = bh.Stop() }() //nolint:errcheck // dont care about error on stop

	// Add global middleware, it will be applied in order of addition
	bh.Use(th.PanicRecovery())
	bh.Use(th.Timeout(config.Common.Timeout))

	bh.Use(tm.AddUpdateSlogAttrs())

	usm := session.NewUserSessionManagerInMem[*session.SimpleUserSession](session.NewSimpleUserSession)
	useStateMiddleware := tm.NewUserSessionMiddleware[*session.SimpleUserSession](usm)
	bh.Use(useStateMiddleware.Middleware())

	cmdHandlerGroup := cmdrouter.NewCommandHandlerGroup()
	cmdHandlerGroup.RegisterCommand("start", cmdh.HandleStart, "Show start-bot message")
	cmdHandlerGroup.RegisterCommand("cancel", cmdh.NewCancelHandler(usm).Handle, "Interrupt any current operation/s")
	cmdHandlerGroup.RegisterCommand("add", cmdh.NewAddHandler(usm).Handle, "Add new note to the list")

	cmdHandlerGroup.BindCommandsToHandler(cmdrouter.NewBotHandler(bh))

	logger.Info("CmdHandlerGroup", "group", cmdHandlerGroup)

	logger.Info("Starting bot ...")
	err = bh.Start()
	if err != nil {
		logger.ErrorContext(ctx, "Failed to start bot", "error", err)
		return
	}
}
