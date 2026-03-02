package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"

	"github.com/aface/sortie/internal/task"
)

type Project struct {
	ID              int64
	Path            string
	Name            string
	DefaultPriority task.Priority
	CreatedAt       time.Time
}

// GetOrCreateProject returns the project for the given path, creating it if needed.
func (db *DB) GetOrCreateProject(projectPath string) (*Project, error) {
	// Normalize path
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		absPath = projectPath
	}

	// Try to find existing
	p, err := db.GetProjectByPath(absPath)
	if err == nil {
		return p, nil
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query project: %w", err)
	}

	// Create new project — derive name from directory basename
	name := filepath.Base(absPath)
	result, err := db.Exec(
		`INSERT INTO projects (path, name) VALUES (?, ?)`,
		absPath, name,
	)
	if err != nil {
		// Race condition: another goroutine may have created it
		p, err2 := db.GetProjectByPath(absPath)
		if err2 == nil {
			return p, nil
		}
		return nil, fmt.Errorf("failed to create project: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Project{
		ID:              id,
		Path:            absPath,
		Name:            name,
		DefaultPriority: task.PriorityMedium,
		CreatedAt:       time.Now(),
	}, nil
}

// GetProjectByPath finds a project by its filesystem path.
func (db *DB) GetProjectByPath(path string) (*Project, error) {
	var p Project
	var defaultPriority sql.NullString
	err := db.QueryRow(
		`SELECT id, path, name, default_priority, created_at FROM projects WHERE path = ?`, path,
	).Scan(&p.ID, &p.Path, &p.Name, &defaultPriority, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if defaultPriority.Valid {
		p.DefaultPriority = task.Priority(defaultPriority.String)
	} else {
		p.DefaultPriority = task.PriorityMedium
	}
	return &p, nil
}

// GetProject returns a project by ID.
func (db *DB) GetProject(id int64) (*Project, error) {
	var p Project
	var defaultPriority sql.NullString
	err := db.QueryRow(
		`SELECT id, path, name, default_priority, created_at FROM projects WHERE id = ?`, id,
	).Scan(&p.ID, &p.Path, &p.Name, &defaultPriority, &p.CreatedAt)
	if err != nil {
		return nil, err
	}
	if defaultPriority.Valid {
		p.DefaultPriority = task.Priority(defaultPriority.String)
	} else {
		p.DefaultPriority = task.PriorityMedium
	}
	return &p, nil
}

// GetProjectsByName returns all projects matching the given name (basename).
func (db *DB) GetProjectsByName(name string) ([]*Project, error) {
	rows, err := db.Query(`SELECT id, path, name, default_priority, created_at FROM projects WHERE name = ? ORDER BY id ASC`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		var p Project
		var defaultPriority sql.NullString
		if err := rows.Scan(&p.ID, &p.Path, &p.Name, &defaultPriority, &p.CreatedAt); err != nil {
			return nil, err
		}
		if defaultPriority.Valid {
			p.DefaultPriority = task.Priority(defaultPriority.String)
		} else {
			p.DefaultPriority = task.PriorityMedium
		}
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}

// ListProjects returns all registered projects.
func (db *DB) ListProjects() ([]*Project, error) {
	rows, err := db.Query(`SELECT id, path, name, default_priority, created_at FROM projects ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []*Project
	for rows.Next() {
		var p Project
		var defaultPriority sql.NullString
		if err := rows.Scan(&p.ID, &p.Path, &p.Name, &defaultPriority, &p.CreatedAt); err != nil {
			return nil, err
		}
		if defaultPriority.Valid {
			p.DefaultPriority = task.Priority(defaultPriority.String)
		} else {
			p.DefaultPriority = task.PriorityMedium
		}
		projects = append(projects, &p)
	}
	return projects, rows.Err()
}
