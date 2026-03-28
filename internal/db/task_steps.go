package db

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CreateTaskStep inserts a new step record with status='running' and started_at=now.
// Uses INSERT OR REPLACE to handle re-runs (e.g., loop iterations).
func (db *DB) CreateTaskStep(taskID int64, stepName string) error {
	_, err := db.Exec(
		`INSERT OR REPLACE INTO task_steps (task_id, step_name, status, started_at)
		 VALUES (?, ?, 'running', ?)`,
		taskID, stepName, time.Now(),
	)
	return err
}

// CompleteTaskStep updates a step record with status='completed', context, exit_code, and completed_at.
func (db *DB) CompleteTaskStep(taskID int64, stepName string, context *string, exitCode int) error {
	_, err := db.Exec(
		`UPDATE task_steps SET status = 'completed', context = ?, exit_code = ?, completed_at = ?
		 WHERE task_id = ? AND step_name = ?`,
		context, exitCode, time.Now(), taskID, stepName,
	)
	return err
}

// GetTaskStepContext returns the context for a single completed step.
func (db *DB) GetTaskStepContext(taskID int64, stepName string) (string, error) {
	var context sql.NullString
	err := db.QueryRow(
		`SELECT context FROM task_steps WHERE task_id = ? AND step_name = ? AND status = 'completed'`,
		taskID, stepName,
	).Scan(&context)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	if context.Valid {
		return context.String, nil
	}
	return "", nil
}

// GetTaskStepContexts returns contexts for multiple steps (replaces CollectArtifacts).
func (db *DB) GetTaskStepContexts(taskID int64, stepNames []string) (map[string]string, error) {
	if len(stepNames) == 0 {
		return make(map[string]string), nil
	}

	placeholders := make([]string, len(stepNames))
	args := make([]any, 0, len(stepNames)+1)
	args = append(args, taskID)
	for i, name := range stepNames {
		placeholders[i] = "?"
		args = append(args, name)
	}

	query := fmt.Sprintf(
		`SELECT step_name, context FROM task_steps
		 WHERE task_id = ? AND step_name IN (%s) AND status = 'completed' AND context IS NOT NULL`,
		strings.Join(placeholders, ","),
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name, ctx string
		if err := rows.Scan(&name, &ctx); err != nil {
			return nil, err
		}
		result[name] = ctx
	}
	return result, rows.Err()
}

// DeleteTaskSteps deletes all step records for a task (used on full retry/reset).
func (db *DB) DeleteTaskSteps(taskID int64) error {
	_, err := db.Exec(`DELETE FROM task_steps WHERE task_id = ?`, taskID)
	return err
}

// DeleteTaskStepsFrom deletes step records from a given index onward (for retry-from-step).
func (db *DB) DeleteTaskStepsFrom(taskID int64, stepNames []string) error {
	if len(stepNames) == 0 {
		return nil
	}
	placeholders := make([]string, len(stepNames))
	args := make([]any, 0, len(stepNames)+1)
	args = append(args, taskID)
	for i, name := range stepNames {
		placeholders[i] = "?"
		args = append(args, name)
	}
	query := fmt.Sprintf(
		`DELETE FROM task_steps WHERE task_id = ? AND step_name IN (%s)`,
		strings.Join(placeholders, ","),
	)
	_, err := db.Exec(query, args...)
	return err
}
