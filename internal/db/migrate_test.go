package db

import (
	"path/filepath"
	"testing"
)

// TestLatestSchemaVersionMatchesMigrationLadder guards the single-sourcing
// this test file is named after: latestSchemaVersion (used to stamp fresh
// installs) is derived from len(migrations) rather than hardcoded, so it
// cannot silently drift from the upgrade ladder the way two independent
// literals could.
func TestLatestSchemaVersionMatchesMigrationLadder(t *testing.T) {
	if len(migrations) == 0 {
		t.Fatal("migrations ladder is empty")
	}
	// migrations[0] upgrades the implicit pre-migration version (1) to
	// version 2, so the last entry's target version is len(migrations)+1.
	want := len(migrations) + 1
	if latestSchemaVersion != want {
		t.Errorf("latestSchemaVersion = %d, want %d (derived from len(migrations)=%d)", latestSchemaVersion, want, len(migrations))
	}
}

// TestMigrateReappliesFromOlderRecordedVersion exercises the actual upgrade
// loop in migrate() (not just the version==0 fresh-install path every other
// test in this package takes): starting from a fully-migrated fresh
// database, roll schema_version back and drop the table the last migration
// creates, then re-run migrate() and confirm it walks forward from the
// rolled-back version, recreates the table via migrateV19, and re-stamps
// schema_version at latestSchemaVersion.
func TestMigrateReappliesFromOlderRecordedVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()

	// Simulate a database that was last migrated at version 18: drop the
	// table migrateV19 creates and roll the recorded version back.
	if _, err := database.sqlDB.Exec(`DROP TABLE task_waits_on`); err != nil {
		t.Fatalf("failed to drop task_waits_on: %v", err)
	}
	if _, err := database.sqlDB.Exec(`UPDATE schema_version SET version = 18`); err != nil {
		t.Fatalf("failed to roll back schema_version: %v", err)
	}

	if err := database.migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var version int
	row := database.sqlDB.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_version`)
	if err := row.Scan(&version); err != nil {
		t.Fatalf("failed to read schema version: %v", err)
	}
	if version != latestSchemaVersion {
		t.Errorf("schema_version after re-migrate = %d, want %d", version, latestSchemaVersion)
	}

	var count int
	row = database.sqlDB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'task_waits_on'`)
	if err := row.Scan(&count); err != nil {
		t.Fatalf("failed to check task_waits_on existence: %v", err)
	}
	if count != 1 {
		t.Errorf("expected migrateV19 to recreate task_waits_on, got count=%d", count)
	}
}
