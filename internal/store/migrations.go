package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"log/slog"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// MigrateUp runs all pending UP migrations embedded in the binary.
// It is idempotent — running it when the schema is already current is safe.
func MigrateUp(ctx context.Context, dsn string, logger *slog.Logger) error {
	m, err := newMigrator(dsn)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("store: migrate up: %w", err)
	}

	version, dirty, err := m.Version()
	if err != nil && !errors.Is(err, migrate.ErrNilVersion) {
		return fmt.Errorf("store: migration version: %w", err)
	}

	logger.InfoContext(ctx, "migrations applied",
		slog.Uint64("version", uint64(version)),
		slog.Bool("dirty", dirty),
	)
	return nil
}

// MigrateDown rolls back a single migration step. Intended for development
// and test environments only; never call this in production automation.
func MigrateDown(ctx context.Context, dsn string, steps int, logger *slog.Logger) error {
	m, err := newMigrator(dsn)
	if err != nil {
		return err
	}
	defer m.Close()

	if err := m.Steps(-steps); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("store: migrate down %d: %w", steps, err)
	}

	logger.WarnContext(ctx, "rolled back migrations", slog.Int("steps", steps))
	return nil
}

// MigrateVersion returns the current schema version and whether the DB is
// in a dirty (partially applied) state.
func MigrateVersion(dsn string) (version uint, dirty bool, err error) {
	m, err := newMigrator(dsn)
	if err != nil {
		return 0, false, err
	}
	defer m.Close()

	v, d, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return 0, false, nil
	}
	return v, d, err
}

func newMigrator(dsn string) (*migrate.Migrate, error) {
	src, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("store: migration source: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", src, dsn)
	if err != nil {
		return nil, fmt.Errorf("store: create migrator: %w", err)
	}
	return m, nil
}
