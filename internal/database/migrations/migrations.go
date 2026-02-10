// Package migrations provides database migration management for tvarr.
// It uses GORM's AutoMigrate with a migration registry to track versions.
package migrations

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"gorm.io/gorm"
)

// Migration represents a single database migration.
type Migration struct {
	Version     string
	Description string
	Up          func(tx *gorm.DB) error
	Down        func(tx *gorm.DB) error
}

// MigrationRecord tracks applied migrations in the database.
type MigrationRecord struct {
	ID          uint      `gorm:"primarykey"`
	Version     string    `gorm:"uniqueIndex;not null"`
	Description string    `gorm:"not null"`
	AppliedAt   time.Time `gorm:"not null"`
}

// TableName returns the table name for migration records.
func (MigrationRecord) TableName() string {
	return "schema_migrations"
}

// Migrator handles database migrations.
type Migrator struct {
	db         *gorm.DB
	logger     *slog.Logger
	migrations []Migration
}

// NewMigrator creates a new Migrator instance.
func NewMigrator(db *gorm.DB, logger *slog.Logger) *Migrator {
	if logger == nil {
		logger = slog.Default()
	}
	return &Migrator{
		db:         db,
		logger:     logger,
		migrations: make([]Migration, 0),
	}
}

// RegisterAll adds multiple migrations to the registry.
func (m *Migrator) RegisterAll(migrations []Migration) {
	m.migrations = append(m.migrations, migrations...)
}

// Init creates the migration tracking table if it doesn't exist.
func (m *Migrator) Init(ctx context.Context) error {
	return m.db.WithContext(ctx).AutoMigrate(&MigrationRecord{})
}

// Up applies all pending migrations.
func (m *Migrator) Up(ctx context.Context) error {
	if err := m.Init(ctx); err != nil {
		return fmt.Errorf("initializing migrations table: %w", err)
	}

	// Sort migrations by version
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})

	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return fmt.Errorf("getting applied migrations: %w", err)
	}

	for _, migration := range m.migrations {
		if applied[migration.Version] {
			continue
		}

		m.logger.InfoContext(ctx, "applying migration",
			slog.String("version", migration.Version),
			slog.String("description", migration.Description),
		)

		if err := m.applyMigration(ctx, migration); err != nil {
			return fmt.Errorf("applying migration %s: %w", migration.Version, err)
		}

		m.logger.InfoContext(ctx, "migration applied",
			slog.String("version", migration.Version),
		)
	}

	return nil
}

// Down rolls back the last applied migration.
func (m *Migrator) Down(ctx context.Context) error {
	if err := m.Init(ctx); err != nil {
		return fmt.Errorf("initializing migrations table: %w", err)
	}

	// Get the last applied migration
	var record MigrationRecord
	if err := m.db.WithContext(ctx).Order("version DESC").First(&record).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			m.logger.InfoContext(ctx, "no migrations to rollback")
			return nil
		}
		return fmt.Errorf("getting last migration: %w", err)
	}

	// Find the migration definition
	var migration *Migration
	for i := range m.migrations {
		if m.migrations[i].Version == record.Version {
			migration = &m.migrations[i]
			break
		}
	}

	if migration == nil {
		return fmt.Errorf("migration definition not found for version %s", record.Version)
	}

	if migration.Down == nil {
		return fmt.Errorf("migration %s does not support rollback", record.Version)
	}

	m.logger.InfoContext(ctx, "rolling back migration",
		slog.String("version", migration.Version),
		slog.String("description", migration.Description),
	)

	if err := m.rollbackMigration(ctx, *migration); err != nil {
		return fmt.Errorf("rolling back migration %s: %w", migration.Version, err)
	}

	m.logger.InfoContext(ctx, "migration rolled back",
		slog.String("version", migration.Version),
	)

	return nil
}

// Status returns the status of all migrations.
func (m *Migrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	if err := m.Init(ctx); err != nil {
		return nil, fmt.Errorf("initializing migrations table: %w", err)
	}

	applied, err := m.appliedRecords(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting applied migrations: %w", err)
	}

	// Sort migrations by version
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})

	statuses := make([]MigrationStatus, 0, len(m.migrations))
	for _, migration := range m.migrations {
		status := MigrationStatus{
			Version:     migration.Version,
			Description: migration.Description,
			Applied:     false,
		}
		if record, ok := applied[migration.Version]; ok {
			status.Applied = true
			status.AppliedAt = &record.AppliedAt
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// MigrationStatus represents the status of a single migration.
type MigrationStatus struct {
	Version     string
	Description string
	Applied     bool
	AppliedAt   *time.Time
}

// Pending returns the list of pending migrations.
func (m *Migrator) Pending(ctx context.Context) ([]Migration, error) {
	if err := m.Init(ctx); err != nil {
		return nil, fmt.Errorf("initializing migrations table: %w", err)
	}

	applied, err := m.appliedVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting applied migrations: %w", err)
	}

	// Sort migrations by version
	sort.Slice(m.migrations, func(i, j int) bool {
		return m.migrations[i].Version < m.migrations[j].Version
	})

	pending := make([]Migration, 0)
	for _, migration := range m.migrations {
		if !applied[migration.Version] {
			pending = append(pending, migration)
		}
	}

	return pending, nil
}

// applyMigration applies a single migration within a transaction.
func (m *Migrator) applyMigration(ctx context.Context, migration Migration) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := migration.Up(tx); err != nil {
			return err
		}

		record := MigrationRecord{
			Version:     migration.Version,
			Description: migration.Description,
			AppliedAt:   time.Now().UTC(),
		}
		return tx.Create(&record).Error
	})
}

// rollbackMigration rolls back a single migration within a transaction.
func (m *Migrator) rollbackMigration(ctx context.Context, migration Migration) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := migration.Down(tx); err != nil {
			return err
		}

		return tx.Where("version = ?", migration.Version).Delete(&MigrationRecord{}).Error
	})
}

// appliedVersions returns a map of applied migration versions.
func (m *Migrator) appliedVersions(ctx context.Context) (map[string]bool, error) {
	var records []MigrationRecord
	if err := m.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}

	applied := make(map[string]bool, len(records))
	for _, record := range records {
		applied[record.Version] = true
	}
	return applied, nil
}

// appliedRecords returns a map of applied migration records.
func (m *Migrator) appliedRecords(ctx context.Context) (map[string]MigrationRecord, error) {
	var records []MigrationRecord
	if err := m.db.WithContext(ctx).Find(&records).Error; err != nil {
		return nil, err
	}

	applied := make(map[string]MigrationRecord, len(records))
	for _, record := range records {
		applied[record.Version] = record
	}
	return applied, nil
}
