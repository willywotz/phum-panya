package db

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Dialector returns the GORM dialector for driver without connecting. For
// "sqlite" (or "") it wraps sqlitePath with the pure-Go glebarez driver and
// WAL pragmas; for "postgres" it uses pgDSN via the pure-Go pgx driver.
func Dialector(driver, sqlitePath, pgDSN string) (gorm.Dialector, error) {
	switch driver {
	case "", "sqlite":
		dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", sqlitePath)
		return sqlite.Open(dsn), nil
	case "postgres":
		return postgres.Open(pgDSN), nil
	default:
		return nil, fmt.Errorf("unknown db driver %q", driver)
	}
}

// OpenWith opens the database for driver and returns a configured *gorm.DB.
func OpenWith(driver, sqlitePath, pgDSN string) (*gorm.DB, error) {
	dial, err := Dialector(driver, sqlitePath, pgDSN)
	if err != nil {
		return nil, err
	}
	g, err := gorm.Open(dial, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", driver, err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	return g, nil
}

// Open opens a SQLite database at path (back-compat wrapper over OpenWith).
func Open(path string) (*gorm.DB, error) {
	return OpenWith("sqlite", path, "")
}

// Tx runs fn inside a transaction. Every query in fn MUST use tx.
func Tx(g *gorm.DB, fn func(tx *gorm.DB) error) error {
	return g.Transaction(fn)
}
