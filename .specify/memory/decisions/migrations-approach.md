# Decision: Database Migration Approach

**Date**: 2025-11-29  
**Status**: Active  
**Context**: Database schema management strategy

## Decision

Use GORM's `AutoMigrate` for database migrations with a versioned migration registry.

## Implementation

Migrations are defined in `internal/database/migrations/registry.go` using the `Migration` struct:

```go
type Migration struct {
    Version     string
    Description string
    Up          func(tx *gorm.DB) error
    Down        func(tx *gorm.DB) error
}
```

Each migration uses `tx.AutoMigrate(&models.Model{})` for the Up function.

## Capabilities

GORM AutoMigrate handles:
- Table creation
- Adding new columns  
- Creating indexes and constraints
- Foreign key relationships
- Database-agnostic DDL (SQLite, PostgreSQL, MySQL)

## Known Limitations

**AutoMigrate does NOT support:**
1. **Dropping columns** - must use raw SQL: `tx.Migrator().DropColumn(&Model{}, "column_name")`
2. **Changing column types** - must use raw SQL: `tx.Migrator().AlterColumn(&Model{}, "column_name")`
3. **Renaming columns** - must use raw SQL: `tx.Migrator().RenameColumn(&Model{}, "old", "new")`
4. **Dropping indexes** - must use: `tx.Migrator().DropIndex(&Model{}, "index_name")`
5. **Complex constraints** - may require database-specific SQL

## Future Schema Changes

For migrations requiring the above operations, use GORM's Migrator interface with database detection:

```go
func migrationXXX() Migration {
    return Migration{
        Version:     "XXX",
        Description: "Alter column example",
        Up: func(tx *gorm.DB) error {
            // Use Migrator for schema changes
            if err := tx.Migrator().AlterColumn(&models.SomeModel{}, "field_name"); err != nil {
                return err
            }
            return nil
        },
        Down: func(tx *gorm.DB) error {
            // Reverse the change
            return tx.Migrator().AlterColumn(&models.SomeModel{}, "field_name")
        },
    }
}
```

For database-specific SQL when needed:

```go
Up: func(tx *gorm.DB) error {
    dialect := tx.Dialector.Name()
    switch dialect {
    case "sqlite":
        // SQLite doesn't support ALTER COLUMN, need table recreation
        return tx.Exec(`...sqlite specific SQL...`).Error
    case "postgres":
        return tx.Exec(`ALTER TABLE foo ALTER COLUMN bar TYPE varchar(500)`).Error
    case "mysql":
        return tx.Exec(`ALTER TABLE foo MODIFY bar VARCHAR(500)`).Error
    default:
        return fmt.Errorf("unsupported dialect: %s", dialect)
    }
}
```

## Alternative Considered

Dedicated migration tools like `golang-migrate/migrate` or `pressly/goose` were considered but rejected because:
- Additional dependency
- GORM's Migrator interface covers most needs
- Current project scope doesn't require complex migrations yet

## References

- GORM Migration docs: https://gorm.io/docs/migration.html
- GORM Migrator interface: https://gorm.io/docs/migration.html#Migrator-Interface
