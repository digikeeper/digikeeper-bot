package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/ilyakaznacheev/cleanenv"

	"github.com/gitrus/digikeeper-bot/pkg/loggingctx"
)

// SecretValue is a string value that may be sourced from a file instead of
// the environment, so secrets never have to be inlined into the process env.
type SecretValue string

// SetValue implements the cleanenv.Setter interface.
func (sv *SecretValue) SetValue(s string) error {
	*sv = SecretValue(s)
	return nil
}

// LoadFromFile replaces the value with the trimmed contents of filePath.
func (sv *SecretValue) LoadFromFile(filePath string) error {
	//nolint:gosec // the path is operator-supplied configuration, not user input
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read secret file %q: %w", filePath, err)
	}

	*sv = SecretValue(strings.TrimSpace(string(data)))
	return nil
}

// String returns the raw secret value.
func (sv SecretValue) String() string {
	return string(sv)
}

// CommonConfig holds process-wide runtime settings.
type CommonConfig struct {
	Env       string        `env:"ENVIRONMENT_NAME" env-default:"dev"`
	Timeout   time.Duration `env:"COMMON_TIMEOUT" env-default:"5s"`
	LocalPort string        `env:"LOCAL_PORT" env-default:"9000"`
	LocalHost string        `env:"LOCAL_HOST" env-default:"localhost"`
}

// TelegramConfig holds bot credentials and update-delivery settings.
type TelegramConfig struct {
	BotKey             SecretValue `env:"BOT_TOKEN" env-default:""`
	BotKeyFile         string      `env:"BOT_TOKEN_FILE" env-default:""`
	PublicURL          string      `env:"BOT_PUBLIC_URL" env-default:"localhost"`
	AllowedUpdates     []string    `env:"ALLOWED_UPDATES" env-default:"message"`
	WebHookSecretToken string      `env:"WEB_HOOK_SECRET_TOKEN"`
}

// SqliteConfig points at the file-backed SQLite database that stores sessions.
type SqliteConfig struct {
	Path string `env:"SQLITE_PATH" env-default:"./sessions.db"`
}

// SessionConfig controls user session lifetime.
// An ExpiresAfter of 0 disables expiry and the background sweeper.
type SessionConfig struct {
	ExpiresAfter time.Duration `env:"SESSION_EXPIRES_AFTER" env-default:"24h"`
}

// Config is the full bot configuration.
type Config struct {
	Common   CommonConfig   `yaml:"common"`
	Telegram TelegramConfig `yaml:"telegram" env-prefix:"TELEGRAM_"`
	Sqlite   SqliteConfig   `yaml:"sqlite"`
	Session  SessionConfig  `yaml:"session"`
}

// IsDevEnv reports whether the bot is running in a development environment.
func (c *Config) IsDevEnv() bool {
	return strings.HasPrefix(strings.ToLower(c.Common.Env), "dev")
}

// resolveSecrets loads file-backed secrets, which take precedence over their
// inline counterparts, and verifies that the required ones are present.
func (c *Config) resolveSecrets() error {
	if c.Telegram.BotKeyFile != "" {
		if err := c.Telegram.BotKey.LoadFromFile(c.Telegram.BotKeyFile); err != nil {
			return fmt.Errorf("read bot token: %w", err)
		}
	}

	if c.Telegram.BotKey == "" {
		return errors.New("no bot token: set TELEGRAM_BOT_TOKEN or TELEGRAM_BOT_TOKEN_FILE")
	}

	return nil
}

// configure builds the Config from the environment, sets up the default
// logger, and terminates the process if the configuration is unusable.
func configure() Config {
	var cfg Config

	if err := cleanenv.ReadEnv(&cfg); err != nil {
		fatal("failed to fetch env vars", err)
	}

	if err := configureLogger(cfg.Common.Env); err != nil {
		fatal("failed to configure logger", err)
	}

	// In dev a local .env is a convenience, not a requirement.
	if cfg.IsDevEnv() {
		if err := cleanenv.ReadConfig(".env", &cfg); err != nil {
			slog.Warn("no .env loaded, using environment only", "error", err)
		}
	}

	if err := cfg.resolveSecrets(); err != nil {
		fatal("failed to resolve secrets", err)
	}

	return cfg
}

// configureLogger sets the default slog logger built by pkg/loggingctx.
func configureLogger(environ string) error {
	logger, err := loggingctx.InitLogger(environ)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	slog.SetDefault(logger)

	return nil
}

// fatal reports an unrecoverable startup error and terminates the process.
func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}
