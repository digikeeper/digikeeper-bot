package migrations

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var fsys embed.FS

// Run applies all migrations to db. It does not manage the db lifecycle: the
// caller owns the connection and decides whether to close it afterwards. It is
// invoked by sqlite.Open on the serving pool, and is idempotent (re-running on
// an up-to-date schema is a no-op).
func Run(db *sql.DB) error {
	source, err := iofs.New(fsys, ".")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}

	driver, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("init migrate sqlite driver: %w", err)
	}

	migrator, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}

	if err := migrator.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}

	return nil
}
