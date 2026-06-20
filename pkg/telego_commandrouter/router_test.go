package telegocommandrouter_test

import (
	"testing"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	tcr "github.com/gitrus/digikeeper-bot/pkg/telego_commandrouter"
)

type MockBotHandler struct {
	mock.Mock
}

func (m *MockBotHandler) Group(predicates ...th.Predicate) tcr.HandlerGroup {
	m.Called(predicates)
	return m
}

func (m *MockBotHandler) Handle(handler th.Handler, predicates ...th.Predicate) {
	m.Called(handler, predicates)
}

func TestNewCommandHandlerGroup(t *testing.T) {
	chg := tcr.NewCommandHandlerGroup()
	assert.NotNil(t, chg, "CommandHandlerGroup should not be nil")
}

func TestRegisterCommand(t *testing.T) {
	chg := tcr.NewCommandHandlerGroup()

	// Create a simple test handler
	testHandler := func(ctx *th.Context, update telego.Update) error { return nil }

	// Register a command
	chg.RegisterCommand("test", testHandler, "Test command description")

	commandToHandler := chg.GetRegisteredCommandsInfo()

	// Verify our test command is in the map
	regCmd, ok := commandToHandler["test"]
	assert.True(t, ok, "The 'test' command should be registered")
	assert.Equal(t, "Test command description", regCmd.Description, "Description should match")
}

func TestBindCommandHandlerGroup(t *testing.T) {
	chg := tcr.NewCommandHandlerGroup()

	mockBotHandler := new(MockBotHandler)
	mockBotHandler.On("Group", mock.Anything).Return(mockBotHandler)
	mockBotHandler.On("Handle", mock.Anything, mock.Anything).Return()

	testHandler := func(ctx *th.Context, update telego.Update) error { return nil }
	chg.RegisterCommand("test", testHandler, "Test command description")

	chg.BindCommandsToHandler(mockBotHandler)

	// A single command group is created for all command handlers, scoped by one
	// predicate (th.AnyCommand()).
	mockBotHandler.AssertNumberOfCalls(t, "Group", 1)
	for _, call := range mockBotHandler.Calls {
		if call.Method != "Group" {
			continue
		}
		predicates := call.Arguments[0].([]th.Predicate)
		assert.Len(t, predicates, 1, "Group should be created with a single predicate")
	}

	// Handlers registered on the group: the debug logger (no predicate) plus the
	// "test", "help" and "unknown" handlers (one predicate each).
	mockBotHandler.AssertNumberOfCalls(t, "Handle", 4)

	var withPredicate, withoutPredicate int
	for _, call := range mockBotHandler.Calls {
		if call.Method != "Handle" {
			continue
		}
		assert.NotNil(t, call.Arguments[0], "handler should not be nil")

		predicates := call.Arguments[1].([]th.Predicate)
		switch len(predicates) {
		case 0:
			withoutPredicate++
		case 1:
			withPredicate++
		default:
			t.Errorf("unexpected predicate count %d", len(predicates))
		}
	}

	assert.Equal(t, 1, withoutPredicate, "debug handler is registered without a predicate")
	assert.Equal(t, 3, withPredicate, "test, help and unknown handlers each have one predicate")
}
