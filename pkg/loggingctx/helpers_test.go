package loggingctx_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/gitrus/digikeeper-bot/pkg/loggingctx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func findAttr(t *testing.T, attrs []any, key string) slog.Attr {
	t.Helper()

	for _, value := range attrs {
		attr, ok := value.(slog.Attr)
		require.True(t, ok, "log attribute has unexpected type %T", value)
		if attr.Key == key {
			return attr
		}
	}

	t.Fatalf("attribute with key %q not found", key)
	return slog.Attr{}
}

func TestAddLogAttr(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    any
		setupFn  func(context.Context) context.Context
		expected int
	}{
		{
			name:     "add first attribute",
			key:      "test-key",
			value:    "test-value",
			setupFn:  func(ctx context.Context) context.Context { return ctx },
			expected: 1,
		},
		{
			name:  "override existing attribute",
			key:   "test-key",
			value: "test-value2",
			setupFn: func(ctx context.Context) context.Context {
				return loggingctx.AddLogAttr(ctx, "test-key", "test-value")
			},
			expected: 1,
		},
		{
			name:  "add additional attribute",
			key:   "test-key",
			value: 64,
			setupFn: func(ctx context.Context) context.Context {
				return loggingctx.AddLogAttr(ctx, "another-key", "another-value")
			},
			expected: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := tc.setupFn(context.Background())
			attrs := loggingctx.GetLogAttrs(loggingctx.AddLogAttr(ctx, tc.key, tc.value))

			assert.Len(t, attrs, tc.expected)
			assert.Equal(t, slog.Any(tc.key, tc.value).Value, findAttr(t, attrs, tc.key).Value)
		})
	}
}

func TestGetLogAttrsWithDifferentContexts(t *testing.T) {
	ctx1 := context.Background()
	ctx1 = loggingctx.AddLogAttr(ctx1, "key1", "value1")

	ctx2 := context.Background()
	ctx2 = loggingctx.AddLogAttr(ctx2, "key2", "value2")

	attrs1 := loggingctx.GetLogAttrs(ctx1)
	attrs2 := loggingctx.GetLogAttrs(ctx2)

	assert.Equal(t, 1, len(attrs1))
	assert.Equal(t, 1, len(attrs2))
	assert.Equal(t, "value1", findAttr(t, attrs1, "key1").Value.Any())
	assert.Equal(t, "value2", findAttr(t, attrs2, "key2").Value.Any())
}

func TestInitLogger(t *testing.T) {
	tests := []struct {
		name        string
		environ     string
		expectError bool
	}{
		{
			name:        "development environment",
			environ:     "dev",
			expectError: false,
		},
		{
			name:        "production environment",
			environ:     "prod",
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			logger, err := loggingctx.InitLogger(tc.environ)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.NotNil(t, logger)
			assert.IsType(t, &slog.Logger{}, logger)
		})
	}
}
