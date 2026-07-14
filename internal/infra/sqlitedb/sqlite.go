// Package sqlite owns the common SQLite specifics: opening, configuring, migrate.
package sqlitedb

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"

	"github.com/gitrus/digikeeper-bot/internal/infra/sqlitedb/migrations"
)

// pragmas are encoded to apply to every connection in the pool.
// busy_timeout softens lock contention, WAL improves read/write
// concurrency, and foreign_keys enforces referential integrity.
const pragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"

// Open open-or-creat a file-backed SQLite database at path and
// applies all migrations, returning a ready-to-use pool.
func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path+"?"+pragmas)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", path, err)
	}
	db.SetMaxOpenConns(1)

	if err := migrations.Run(db); err != nil {
		migrationErr := fmt.Errorf("migrate sqlite %q: %w", path, err)
		if closeErr := db.Close(); closeErr != nil {
			return nil, errors.Join(migrationErr, fmt.Errorf("close sqlite %q after migration failure: %w", path, closeErr))
		}
		return nil, migrationErr
	}
	return db, nil
}

func Close(db *sql.DB) error {
	return db.Close()
}
