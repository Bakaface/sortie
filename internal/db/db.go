package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed schema.sql
var schema string

type DB struct {
	*sql.DB
	path string
}

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	sqlDB, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=ON")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// SQLite is single-writer; limit connections to prevent contention
	sqlDB.SetMaxOpenConns(1)

	db := &DB{DB: sqlDB, path: path}

	if err := db.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func (db *DB) migrate() error {
	// Create schema version tracking
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (version INTEGER NOT NULL)`)
	if err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	var version int
	row := db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&version); err != nil {
		return fmt.Errorf("failed to read schema version: %w", err)
	}

	if version == 0 {
		// Fresh database — apply full schema
		if _, err := db.Exec(schema); err != nil {
			return fmt.Errorf("failed to apply schema: %w", err)
		}
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (1)`); err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Future migrations go here as version checks:
	// if version < 2 { ... migrate ... UPDATE schema_version SET version = 2 }

	return nil
}

func (db *DB) Path() string {
	return db.path
}
