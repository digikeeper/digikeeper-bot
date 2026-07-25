package telegomiddleware_test

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/mymmrac/telego"
	th "github.com/mymmrac/telego/telegohandler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gitrus/digikeeper-bot/pkg/loggingctx"
	tm "github.com/gitrus/digikeeper-bot/pkg/telego_middleware"
)

// helloInput is the sample input shared by the FirstNRunes cases.
const helloInput = "hello"

// TestFirstNRunes verifies FirstNRunes.
func TestFirstNRunes(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		n        int
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			n:        5,
			expected: "",
		},
		{
			name:     "string shorter than n",
			input:    helloInput,
			n:        10,
			expected: helloInput,
		},
		{
			name:     "string longer than n",
			input:    "hello world",
			n:        5,
			expected: helloInput,
		},
		{
			name:     "string with unicode characters",
			input:    "こんにちは世界",
			n:        3,
			expected: "こんに",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := tm.FirstNRunes(tc.input, tc.n)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAddSlogAttrsTyping verifies AddUpdateSlogAttrs returns a handler.
func TestAddSlogAttrsTyping(t *testing.T) {
	handler := tm.AddUpdateSlogAttrs()
	assert.NotNil(t, handler, "AddSlogAttrs should return a non-nil handler")

	handlerType := assert.IsType(
		t,
		th.Handler(nil),
		handler,
		"AddSlogAttrs should return a th.Handler",
	)
	assert.True(t, handlerType, "Handler should be of type th.Handler")
}

func TestAddSlogAttrsHandle(t *testing.T) {
	token := "1234567890:aaaabbbbaaaabbbbaaaabbbbaaaabbbbccc"
	bot, err := telego.NewBot(token)
	require.NoError(t, err)
	updates := make(chan telego.Update, 10)

	bh, err := th.NewBotHandler(bot, updates)
	require.NoError(t, err)

	wg := sync.WaitGroup{}

	wg.Add(1)
	handlerCalled := false
	handler := func(ctx *th.Context, _ telego.Message) error {
		defer wg.Done()
		handlerCalled = true

		attrs := loggingctx.GetLogAttrs(ctx)
		assert.NotEmpty(t, attrs, "attrs persistence error")
		assert.Len(t, attrs, 5, "expected 5 attributes in loggingctx")

		attrsMap := make(map[string]any)
		for _, attr := range attrs {
			slogAttr, ok := attr.(slog.Attr)
			require.True(t, ok, "log attribute has unexpected type %T", attr)
			attrsMap[slogAttr.Key] = slogAttr.Value.Any()
		}

		assert.Equal(t, int64(999), attrsMap["update_id"], "update_id should match expected value")
		assert.Equal(t, int64(123), attrsMap["message_id"], "message_id should match expected value")
		assert.Equal(t, "Test messa", attrsMap["text_first10"], "text_first10 should match expected value")
		assert.Equal(t, int64(456), attrsMap["chat_id"], "chat_id should match expected value")
		assert.Equal(t, int64(789), attrsMap["user_id"], "user_id should match expected value")

		return nil
	}

	bh.Use(tm.AddUpdateSlogAttrs())
	bh.HandleMessage(handler)

	startErr := make(chan error, 1)
	go func() {
		startErr <- bh.Start()
	}()

	testUpdate := telego.Update{
		UpdateID: 999,
		Message: &telego.Message{
			MessageID: 123,
			Text:      "Test message",
			Chat:      telego.Chat{ID: 456},
			From:      &telego.User{ID: 789},
		},
	}
	updates <- testUpdate

	close(updates)
	wg.Wait()
	require.NoError(t, bh.Stop())
	require.NoError(t, <-startErr)

	assert.True(t, handlerCalled, "Handler should have been called")
}
