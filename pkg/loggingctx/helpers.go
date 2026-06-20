// Package loggingctx provides structured logging support through context propagation.
//
// This package includes:
// - adding slog attributes to a context
// - retrieving slog attributes from a context
// - initializing zap logger for slog with automatic context attribute extraction
//
// Examples:
//
//	// Add attributes to context
//	ctx = loggingctx.AddLogAttr(ctx, "user_id", userId)
//	ctx = loggingctx.AddLogAttr(ctx, "request_id", requestId)
//
//	// Initialize zap logger for slog with context attribute support
//	logger, err := loggingctx.InitLogger("dev")
//	if err != nil {
//		// handle error
//	}
//	slog.SetDefault(logger)
//
//	// Later in the request flow, context attributes are automatically included
//	// No need to manually pass GetLogAttrs
//	slog.InfoContext(ctx, "User action completed")
package loggingctx

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
	"go.uber.org/zap/zapcore"
)

type contextKey string

// LogAttrsKey is the context key used for storing logging attributes
const LogAttrsKey contextKey = "LogAttrsKey"

// AddLogAttr adds a logging attribute to the provided context
func AddLogAttr(ctx context.Context, key string, value any) context.Context {
	return AddLogAttrs(ctx, []slog.Attr{slog.Any(key, value)})
}

// AddLogAttr adds a logging attributes list to the provided context
func AddLogAttrs(ctx context.Context, attrs []slog.Attr) context.Context {
	logAttrs, ok := ctx.Value(LogAttrsKey).([]slog.Attr)
	if !ok {
		logAttrs = make([]slog.Attr, 0, 9)
	}

	// Copy existing attributes so we never mutate the slice stored in the
	// parent context, then track where each key lives to allow in-place override.
	result := make([]slog.Attr, len(logAttrs), len(logAttrs)+len(attrs))
	copy(result, logAttrs)

	indexByKey := make(map[string]int, len(result))
	for i, attr := range result {
		indexByKey[attr.Key] = i
	}

	for _, attr := range attrs {
		if i, ok := indexByKey[attr.Key]; ok {
			result[i] = attr
			continue
		}
		indexByKey[attr.Key] = len(result)
		result = append(result, attr)
	}

	return context.WithValue(ctx, LogAttrsKey, result)
}

// GetLogAttrs retrieves logging attributes slice from the context
func GetLogAttrs(ctx context.Context) []any {
	attrs, ok := ctx.Value(LogAttrsKey).([]slog.Attr)
	if !ok {
		return []any{}
	}

	anyattr := make([]any, len(attrs))
	for i, attr := range attrs {
		anyattr[i] = attr
	}
	return anyattr
}

// ContextHandler is a custom slog handler that
// includes context attributes from LogAttrsKey
type ContextHandler struct {
	handler slog.Handler
}

// Handle implements slog.Handler.Handle by adding attributes from context automatically
func (h *ContextHandler) Handle(ctx context.Context, record slog.Record) error {
	if ctx == nil {
		return h.handler.Handle(ctx, record)
	}

	attrs, ok := ctx.Value(LogAttrsKey).([]slog.Attr)
	if ok && len(attrs) > 0 {
		for _, attr := range attrs {
			record.AddAttrs(attr)
		}
	}

	return h.handler.Handle(ctx, record)
}

func (h *ContextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &ContextHandler{handler: h.handler.WithAttrs(attrs)}
}

func (h *ContextHandler) WithGroup(name string) slog.Handler {
	return &ContextHandler{handler: h.handler.WithGroup(name)}
}

func (h *ContextHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.handler.Enabled(ctx, level)
}

func InitLogger(environ string) (*slog.Logger, error) {
	// environ in [dev, <any-else>]
	var logger *zap.Logger
	var err error
	if strings.HasPrefix(environ, "dev") {
		config := zap.Config{
			Encoding:         "json", // Use JSON encoding
			Level:            zap.NewAtomicLevelAt(zap.InfoLevel),
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
			EncoderConfig: zapcore.EncoderConfig{
				TimeKey:      "time",
				LevelKey:     "level",
				MessageKey:   "msg",
				EncodeTime:   zapcore.ISO8601TimeEncoder,
				EncodeLevel:  zapcore.CapitalLevelEncoder,
				CallerKey:    "caller",
				EncodeCaller: zapcore.ShortCallerEncoder,
			},
		}
		logger, err = config.Build()
	} else {
		logger, err = zap.NewProduction()
	}

	if err != nil {
		return nil, fmt.Errorf("Fail at init zap logger %w", err)
	}

	zapHandler := zapslog.NewHandler(logger.Core())
	contextHandler := &ContextHandler{handler: zapHandler}
	slogLogger := slog.New(contextHandler)

	return slogLogger, nil
}
