package main

import (
	"context"
	"log/slog"

	th "github.com/mymmrac/telego/telegohandler"

	cmdh "github.com/gitrus/digikeeper-bot/internal/handler"
	"github.com/gitrus/digikeeper-bot/internal/infra/sessionstore"
	"github.com/gitrus/digikeeper-bot/internal/infra/sqlitedb"
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

	db, err := sqlitedb.Open(config.Sqlite.Path)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to open sqlite", "error", err)
		return
	}
	defer func() { _ = sqlitedb.Close(db) }() //nolint:errcheck // best-effort close on shutdown

	usm, err := sessionstore.New(db, session.NewSimpleUserSession, config.Session.ExpiresAfter)
	if err != nil {
		logger.ErrorContext(ctx, "Failed to init session store", "error", err)
		return
	}

	if config.Session.ExpiresAfter > 0 {
		sweeper := sessionstore.NewSweeper(usm, config.Session.ExpiresAfter)
		go sweeper.Run(ctx)
	}

	useStateMiddleware := tm.NewUserSessionMiddleware(usm)
	bh.Use(useStateMiddleware.Middleware())

	cmdHandlerGroup := cmdrouter.NewCommandHandlerGroup()
	cmdHandlerGroup.RegisterCommand("start", cmdh.HandleStart, "Show start-bot message")
	cmdHandlerGroup.RegisterCommand("cancel", cmdh.NewCancelHandler(usm).Handle, "Interrupt any current operation/s")
	cmdHandlerGroup.RegisterCommand("add", cmdh.NewAddHandler(usm).Handle, "Add new note to the list")

	cmdHandlerGroup.BindCommandsToHandler(ctx, cmdrouter.NewBotHandler(bh))

	logger.InfoContext(ctx, "CmdHandlerGroup", "group", cmdHandlerGroup)

	logger.InfoContext(ctx, "Starting bot ...")
	err = bh.Start()
	if err != nil {
		logger.ErrorContext(ctx, "Failed to start bot", "error", err)
		return
	}
}
