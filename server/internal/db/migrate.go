package db

import (
	"database/sql"
	"errors"
	"log/slog"

	_ "embed"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"io/fs"
	"os"
	"path/filepath"
	"runtime"
)

// RunMigrations applies all pending UP migrations.
// ErrNoChange is swallowed — it is not an error.
func RunMigrations(db *sql.DB, log *slog.Logger) error {
	migrationsFS, err := migrationsDir()
	if err != nil {
		return err
	}

	src, err := iofs.New(migrationsFS, ".")
	if err != nil {
		return err
	}

	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return err
	}

	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}

	log.Info("db migrations applied")
	return nil
}

// migrationsDir locates the db/migrations directory relative to this source file.
// Works both in development (source tree) and when built as a binary (uses os.Executable).
func migrationsDir() (fs.FS, error) {
	// Try relative to this file's source location (works during `go run`).
	_, filename, _, ok := runtime.Caller(0)
	if ok {
		candidate := filepath.Join(filepath.Dir(filename), "..", "..", "db", "migrations")
		if _, err := os.Stat(candidate); err == nil {
			return os.DirFS(candidate), nil
		}
	}

	// Fallback: relative to the working directory.
	candidate := filepath.Join("db", "migrations")
	if _, err := os.Stat(candidate); err == nil {
		return os.DirFS(candidate), nil
	}

	return nil, errors.New("cannot locate db/migrations directory")
}
