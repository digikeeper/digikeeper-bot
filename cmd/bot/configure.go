package main

import (
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/gitrus/digikeeper-bot/pkg/loggingctx"
	"github.com/ilyakaznacheev/cleanenv"
)

// SecretValue is a string value that should be loaded from a file
type SecretValue string

// SetValue implements the cleanenv.Setter interface
func (sv *SecretValue) SetValue(s string) error {
	*sv = SecretValue(s)
	return nil
}

func (sv *SecretValue) LoadFromFile(filePath string) error {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("secret file does not exist: %s", filePath)
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read secret file: %w", err)
	}

	*sv = SecretValue(strings.TrimSpace(string(data)))
	return nil
}

func (sv SecretValue) String() string {
	return string(sv)
}

type CommonConfig struct {
	Env       string        `env:"ENVIRONMENT_NAME" env-default:"dev"`
	Timeout   time.Duration `env:"COMMON_TIMEOUT" env-default:"5s"`
	LocalPort string        `env:"LOCAL_PORT" env-default:"9000"`
	LocalHost string        `env:"LOCAL_HOST" env-default:"localhost"`
}

type TelegramConfig struct {
	BotKey         SecretValue `env:"BOT_TOKEN" env-default:""`
	BotKeyFile     string      `env:"BOT_TOKEN_FILE" env-default:""`
	PublicURL      string      `env:"BOT_PUBLIC_URL" env-default:"localhost"`
	AllowedUpdates []string    `env:"ALLOWED_UPDATES" env-default:"message"`
}

type PostgresConfig struct {
	Enabled      bool        `env:"ENABLED"`
	Host         string      `env:"HOST" env-default:"localhost"`
	Port         string      `env:"PORT" env-default:"5432"`
	Database     string      `env:"DB" env-default:"digikeeper"`
	User         SecretValue `env:"USER" env-default:"postgres"`
	UserFile     string      `env:"USER_FILE" env-default:""`
	Password     SecretValue `env:"PASSWORD" env-default:""`
	PasswordFile string      `env:"PASSWORD_FILE" env-default:""`
}

type DigikeeperLogConfig struct {
	Enabled    bool          `env:"ENABLED"`
	BaseURL    string        `env:"BASE_URL" env-default:"http://localhost:8080"`
	Timeout    time.Duration `env:"TIMEOUT" env-default:"3s"`
	MaxRetries int           `env:"MAX_RETRIES" env-default:"3"`
	ClientID   string        `env:"CLIENT_ID" env-default:"digikeeper-bot"`
	Token      SecretValue   `env:"TOKEN" env-default:""`
	TokenFile  string        `env:"TOKEN_FILE" env-default:""`
}

type Config struct {
	Common        CommonConfig        `yaml:"common"`
	Telegram      TelegramConfig      `yaml:"telegram" env-prefix:"TELEGRAM_"`
	Postgres      PostgresConfig      `yaml:"postgres" env-prefix:"POSTGRES_"`
	DigikeeperLog DigikeeperLogConfig `yaml:"digikeeperlog" env-prefix:"DIGIKEEPER_LOG_"`
}

func (c *Config) IsDevEnv() bool {
	return strings.HasPrefix(strings.ToLower(c.Common.Env), "dev")
}

func configure() Config {
	var cfg Config

	err := cleanenv.ReadEnv(&cfg)
	if err != nil {
		log.Fatalf("Failed to fetch env vars: %v", err)
	}

	err = configureLogger(cfg.Common.Env)
	if err != nil {
		log.Fatalf("Failed to configure logger: %v", err)
	}

	if cfg.IsDevEnv() {
		err = cleanenv.ReadConfig(".env", &cfg)
		if err != nil {
			log.Printf("Failed to get .env: %v", err)
		}
	}

	err = cfg.Telegram.BotKey.LoadFromFile(cfg.Telegram.BotKeyFile)
	if err != nil {
		log.Fatalf("Failed to read bot token: %v", err)
	}

	if cfg.Postgres.Enabled {
		err = cfg.Postgres.User.LoadFromFile(cfg.Postgres.UserFile)
		if err != nil {
			log.Fatalf("Failed to read postgres user: %v", err)
		}

		err = cfg.Postgres.Password.LoadFromFile(cfg.Postgres.PasswordFile)
		if err != nil {
			log.Fatalf("Failed to read postgres password: %v", err)
		}
	}

	if cfg.DigikeeperLog.Enabled && cfg.DigikeeperLog.TokenFile != "" {
		err = cfg.DigikeeperLog.Token.LoadFromFile(cfg.DigikeeperLog.TokenFile)
		if err != nil {
			log.Fatalf("Failed to read digikeeper-log token: %v", err)
		}
	}

	return cfg
}

// configureLogger SetDefault logger from pkg/loggingctx
func configureLogger(environ string) error {
	logger, err := loggingctx.InitLogger(environ)
	if err != nil {
		return fmt.Errorf("failed to initialize logger: %w", err)
	}

	slog.SetDefault(logger)

	return nil
}
