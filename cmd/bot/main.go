package main

import (
	"context"
	"log/slog"

	th "github.com/mymmrac/telego/telegohandler"

	cmdh "github.com/gitrus/digikeeper-bot/internal/cmd_handler"
	"github.com/gitrus/digikeeper-bot/pkg/journal"
	session "github.com/gitrus/digikeeper-bot/pkg/sessionmanager"
	cmdrouter "github.com/gitrus/digikeeper-bot/pkg/telego_commandrouter"
	tm "github.com/gitrus/digikeeper-bot/pkg/telego_middleware"
)

func main() {
	config := configure()
	logger := slog.Default()

	ctx := context.Background()

	var (
		events *journal.Client
		err    error
	)
	if config.DigikeeperLog.Enabled {
		events, err = initJournal(ctx, config, logger)
		if err != nil {
			logger.ErrorContext(ctx, "Failed to init journal client", "error", err)
			return
		}
		defer func() { _ = events.Close() }()
	}

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
	cmdHandlerGroup.RegisterCommand("cancel", cmdh.HandleCancel(usm), "Interrupt any current operation/s")
	cmdHandlerGroup.RegisterCommand("add", cmdh.HandleAdd(usm), "Add new note to the list")

	cmdHandlerGroup.BindCommandsToHandler(bh)

	// Plain text messages (non-command) go to the note-submission handler so
	// that users in the "add" state can finish the /add flow by typing the
	// note contents. Registered after BindCommandsToHandler so command
	// predicates match first.
	bh.Handle(cmdh.HandleAddNoteText(usm, events),
		th.AnyMessageWithText(),
		th.Not(th.AnyCommand()),
	)

	logger.Info("CmdHandlerGroup", "group", cmdHandlerGroup)

	logger.Info("Starting bot ...")
	err = bh.Start()
	if err != nil {
		logger.ErrorContext(ctx, "Failed to start bot", "error", err)
		return
	}
}

// initJournal constructs the journal client from the bot config. Callers
// should only invoke it when DigikeeperLog.Enabled is true.
func initJournal(ctx context.Context, cfg Config, logger *slog.Logger) (*journal.Client, error) {
	jcfg := journal.Config{
		BaseURL:    cfg.DigikeeperLog.BaseURL,
		Timeout:    cfg.DigikeeperLog.Timeout,
		MaxRetries: cfg.DigikeeperLog.MaxRetries,
		ClientID:   cfg.DigikeeperLog.ClientID,
		Token:      cfg.DigikeeperLog.Token.String(),
	}
	return journal.New(ctx, jcfg, journal.WithLogger(logger))
}
