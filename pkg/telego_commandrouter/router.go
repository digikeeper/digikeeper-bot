package telegocommandrouter

import (
	"log/slog"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
)

// HandlerGroup is an interface that defines the Handle method
type HandlerGroup interface {
	Handle(handler th.Handler, predicates ...th.Predicate)
}

// BotHandler is an interface that defines the Group method. It returns a
// HandlerGroup (rather than the concrete *th.HandlerGroup) so callers can be
// unit tested with a mock.
type BotHandler interface {
	Group(predicates ...th.Predicate) HandlerGroup
}

// telegoBotHandler adapts telego's *th.BotHandler to the BotHandler interface.
// telego's Group returns the concrete *th.HandlerGroup, which already satisfies
// HandlerGroup, so the adapter only needs to widen the return type.
type telegoBotHandler struct {
	*th.BotHandler
}

func (a telegoBotHandler) Group(predicates ...th.Predicate) HandlerGroup {
	return a.BotHandler.Group(predicates...)
}

// NewBotHandler adapts a telego *BotHandler to the BotHandler interface.
func NewBotHandler(bh *th.BotHandler) BotHandler {
	return telegoBotHandler{bh}
}

type RegisteredHandler struct {
	Handler     th.Handler
	Description string
}

type CommandHandlerGroup struct {
	commands map[string]*RegisteredHandler
}

func NewCommandHandlerGroup() *CommandHandlerGroup {
	chg := &CommandHandlerGroup{
		commands: make(map[string]*RegisteredHandler),
	}

	return chg
}

func (ch *CommandHandlerGroup) RegisterCommand(command string, handler th.Handler, description string) {
	ch.commands[command] = &RegisteredHandler{Handler: handler, Description: description}
}

// BindCommandsToHandler creates a group of commands and binds them to the bot handler,
// with a default handler for `unknown` commands, and default `/help` command.
// Example:
// ch := NewCommandHandlerGroup()
// ch.RegisterCommand("start", HandleStartCommand, "Start the bot")
// ch.RegisterCommand("stop", HandleStopCommand, "Stop the bot")
// ch.BindCommandsToHandler(bh)
func (ch *CommandHandlerGroup) BindCommandsToHandler(bh BotHandler) {
	slog.Info("Binding command handlers", "commands", ch.getCommandToDescription())

	commands := bh.Group(th.AnyCommand())

	// Add a debug handler to log all incoming commands
	commands.Handle(func(ctx *th.Context, update telego.Update) error {
		if update.Message != nil && update.Message.Text != "" {
			slog.DebugContext(
				ctx.Context(), "Command received", "text", update.Message.Text,
			)
		}
		return ctx.Next(update)
	})

	predicates := make([]th.Predicate, 0, len(ch.commands))
	for command, rh := range ch.commands {
		p := th.CommandEqual(command)
		// Wrap each handler with debug logging
		handlerWithLogging := func(originalHandler th.Handler) th.Handler {
			return func(ctx *th.Context, update telego.Update) error {
				slog.DebugContext(
					ctx.Context(), "Handling command", "command", command,
				)
				return originalHandler(ctx, update)
			}
		}(rh.Handler)
		commands.Handle(handlerWithLogging, p)
		predicates = append(predicates, p)
	}

	helpP := th.CommandEqual("help")
	commands.Handle(
		NewHelpHandler(ch.getCommandToDescription()), th.CommandEqual("help"),
	)
	predicates = append(predicates, helpP)

	slog.Info("Bound predicates", slog.Int("predicates_len", len(predicates)))

	commands.Handle(
		NewUnknownCommandHandler(DefaultUnknownCommandMessage), th.AnyCommand(),
	)
}

func (ch *CommandHandlerGroup) GetRegisteredCommandsInfo() map[string]RegisteredHandler {
	// Create a copy of the map to prevent modification of the original
	commandsCopy := make(map[string]RegisteredHandler)
	for k, v := range ch.commands {
		commandsCopy[k] = *v
	}

	return commandsCopy
}

func (ch *CommandHandlerGroup) getCommandToDescription() map[string]string {
	cmdToDesc := make(map[string]string)
	for command, handler := range ch.commands {
		cmdToDesc[command] = handler.Description
	}
	return cmdToDesc
}
