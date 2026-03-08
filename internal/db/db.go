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
		if _, err := db.Exec(`INSERT INTO schema_version (version) VALUES (11)`); err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
		return nil
	}

	// Migration version 2: Add workflow column
	if version < 2 {
		_, err := db.Exec(`ALTER TABLE tasks ADD COLUMN workflow TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add workflow column: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 2`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 3: Add context column
	if version < 3 {
		_, err := db.Exec(`ALTER TABLE tasks ADD COLUMN context TEXT`)
		if err != nil {
			return fmt.Errorf("failed to add context column: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 3`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 4: Rename status values
	if version < 4 {
		_, err := db.Exec(`UPDATE tasks SET status = 'init' WHERE status = 'generating-title'`)
		if err != nil {
			return fmt.Errorf("failed to rename generating-title status: %w", err)
		}
		_, err = db.Exec(`UPDATE tasks SET status = 'awaiting-approval' WHERE status = 'awaiting_approval'`)
		if err != nil {
			return fmt.Errorf("failed to rename awaiting_approval status: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 4`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 5: Add projects table and project_id to tasks
	if version < 5 {
		_, err := db.Exec(`CREATE TABLE IF NOT EXISTS projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`)
		if err != nil {
			return fmt.Errorf("failed to create projects table: %w", err)
		}

		// Add project_id column (nullable initially for migration)
		_, err = db.Exec(`ALTER TABLE tasks ADD COLUMN project_id INTEGER REFERENCES projects(id)`)
		if err != nil {
			return fmt.Errorf("failed to add project_id column: %w", err)
		}

		// Create a default project for existing tasks
		_, err = db.Exec(`INSERT OR IGNORE INTO projects (path, name) VALUES ('unknown', 'unknown')`)
		if err != nil {
			return fmt.Errorf("failed to create default project: %w", err)
		}

		// Assign all existing tasks to the default project
		_, err = db.Exec(`UPDATE tasks SET project_id = (SELECT id FROM projects WHERE path = 'unknown') WHERE project_id IS NULL`)
		if err != nil {
			return fmt.Errorf("failed to assign tasks to default project: %w", err)
		}

		_, err = db.Exec(`CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id)`)
		if err != nil {
			return fmt.Errorf("failed to create project_id index: %w", err)
		}

		_, err = db.Exec(`UPDATE schema_version SET version = 5`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 6: Add images column
	if version < 6 {
		_, err := db.Exec(`ALTER TABLE tasks ADD COLUMN images TEXT`)
		if err != nil {
			return fmt.Errorf("failed to add images column: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 6`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 7: Add priority to tasks and default_priority to projects
	if version < 7 {
		_, err := db.Exec(`ALTER TABLE tasks ADD COLUMN priority TEXT NOT NULL DEFAULT 'medium'`)
		if err != nil {
			return fmt.Errorf("failed to add priority column to tasks: %w", err)
		}
		_, err = db.Exec(`ALTER TABLE projects ADD COLUMN default_priority TEXT NOT NULL DEFAULT 'medium'`)
		if err != nil {
			return fmt.Errorf("failed to add default_priority column to projects: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 7`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 8: Add loop_iteration to tasks
	if version < 8 {
		_, err := db.Exec(`ALTER TABLE tasks ADD COLUMN loop_iteration INTEGER NOT NULL DEFAULT 0`)
		if err != nil {
			return fmt.Errorf("failed to add loop_iteration column: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 8`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 9: Add branch_name to tasks (user-provided branch template)
	if version < 9 {
		_, err := db.Exec(`ALTER TABLE tasks ADD COLUMN branch_name TEXT NOT NULL DEFAULT ''`)
		if err != nil {
			return fmt.Errorf("failed to add branch_name column: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 9`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 10: Add worktree boolean to tasks
	if version < 10 {
		_, err := db.Exec(`ALTER TABLE tasks ADD COLUMN worktree INTEGER NOT NULL DEFAULT 1`)
		if err != nil {
			return fmt.Errorf("failed to add worktree column: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 10`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	// Migration version 11: Add default_worktree to projects
	if version < 11 {
		_, err := db.Exec(`ALTER TABLE projects ADD COLUMN default_worktree INTEGER NOT NULL DEFAULT 1`)
		if err != nil {
			return fmt.Errorf("failed to add default_worktree column to projects: %w", err)
		}
		_, err = db.Exec(`UPDATE schema_version SET version = 11`)
		if err != nil {
			return fmt.Errorf("failed to set schema version: %w", err)
		}
	}

	return nil
}

func (db *DB) Path() string {
	return db.path
}
