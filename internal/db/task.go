package db

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/aface/ralph-tamer-kit/internal/task"
)

func (db *DB) CreateTask(projectID int64, title, description, slug, workflow, branch string, status task.Status) (*task.Task, error) {
	result, err := db.Exec(
		`INSERT INTO tasks (project_id, title, description, slug, workflow, branch, status) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		projectID, title, description, slug, workflow, branch, status,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return db.GetTask(id)
}

const taskColumns = `id, project_id, title, description, slug, workflow, status, step_index, current_step,
	branch, worktree_path, exit_code, error_message, context,
	created_at, started_at, completed_at, updated_at`

func (db *DB) GetTask(id int64) (*task.Task, error) {
	row := db.QueryRow(fmt.Sprintf(`SELECT %s FROM tasks WHERE id = ?`, taskColumns), id)
	t, err := scanTask(row)
	if err != nil {
		return nil, err
	}

	deps, err := db.getBlockedBy(id)
	if err != nil {
		return nil, err
	}
	t.BlockedBy = deps

	return t, nil
}

func (db *DB) getBlockedBy(taskID int64) ([]int64, error) {
	rows, err := db.Query(`SELECT blocked_by FROM task_dependencies WHERE task_id = ?`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []int64
	for rows.Next() {
		var dep int64
		if err := rows.Scan(&dep); err != nil {
			return nil, err
		}
		deps = append(deps, dep)
	}
	return deps, rows.Err()
}

func (db *DB) GetPendingTasks() ([]*task.Task, error) {
	return db.getTasksByStatus(task.StatusPending)
}

func (db *DB) GetRunningTasks() ([]*task.Task, error) {
	return db.getTasksByStatus(task.StatusRunning)
}

func (db *DB) GetClaimableTasks() ([]*task.Task, error) {
	rows, err := db.Query(fmt.Sprintf(`
		SELECT %s FROM tasks
		WHERE status = ?
		AND id NOT IN (
			SELECT td.task_id FROM task_dependencies td
			JOIN tasks dep ON dep.id = td.blocked_by
			WHERE dep.status != ?
		)
		ORDER BY id ASC
	`, taskColumns), task.StatusPending, task.StatusCompleted)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

func (db *DB) GetAllTasks() ([]*task.Task, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT %s FROM tasks ORDER BY id DESC`, taskColumns))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}

	// Load dependencies for all tasks
	for _, t := range tasks {
		deps, err := db.getBlockedBy(t.ID)
		if err != nil {
			return nil, err
		}
		t.BlockedBy = deps
	}

	return tasks, nil
}

// GetTasksByProject returns all tasks for a specific project.
func (db *DB) GetTasksByProject(projectID int64) ([]*task.Task, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT %s FROM tasks WHERE project_id = ? ORDER BY id DESC`, taskColumns), projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tasks, err := scanTasks(rows)
	if err != nil {
		return nil, err
	}

	for _, t := range tasks {
		deps, err := db.getBlockedBy(t.ID)
		if err != nil {
			return nil, err
		}
		t.BlockedBy = deps
	}

	return tasks, nil
}

func (db *DB) getTasksByStatus(status task.Status) ([]*task.Task, error) {
	rows, err := db.Query(fmt.Sprintf(`
		SELECT %s FROM tasks WHERE status = ? ORDER BY id ASC
	`, taskColumns), status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanTasks(rows)
}

// ClaimTask atomically transitions a task from pending to running.
// Returns true if the claim succeeded, false if the task was not in pending state.
func (db *DB) ClaimTask(id int64) (bool, error) {
	now := time.Now()
	result, err := db.Exec(
		"UPDATE tasks SET status = ?, started_at = ?, updated_at = ? WHERE id = ? AND status = ?",
		task.StatusRunning, now, now, id, task.StatusPending,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (db *DB) UpdateTaskStatus(id int64, status task.Status) error {
	now := time.Now()
	var query string
	var args []interface{}

	switch status {
	case task.StatusRunning:
		query = "UPDATE tasks SET status = ?, started_at = ?, updated_at = ? WHERE id = ?"
		args = []interface{}{status, now, now, id}
	case task.StatusCompleted, task.StatusFailed:
		query = "UPDATE tasks SET status = ?, completed_at = ?, updated_at = ? WHERE id = ?"
		args = []interface{}{status, now, now, id}
	default:
		query = "UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?"
		args = []interface{}{status, now, id}
	}

	_, err := db.Exec(query, args...)
	return err
}

func (db *DB) UpdateTaskWorktreePath(id int64, worktreePath string) error {
	_, err := db.Exec(
		"UPDATE tasks SET worktree_path = ?, updated_at = ? WHERE id = ?",
		worktreePath, time.Now(), id,
	)
	return err
}

func (db *DB) ClearWorktreePath(id int64) error {
	_, err := db.Exec(
		"UPDATE tasks SET worktree_path = NULL, updated_at = ? WHERE id = ?",
		time.Now(), id,
	)
	return err
}

func (db *DB) UpdateTaskStep(id int64, stepIndex int, currentStep string) error {
	_, err := db.Exec(
		"UPDATE tasks SET step_index = ?, current_step = ?, updated_at = ? WHERE id = ?",
		stepIndex, currentStep, time.Now(), id,
	)
	return err
}

func (db *DB) UpdateTaskExitCode(id int64, exitCode int, errorMessage string) error {
	_, err := db.Exec(
		"UPDATE tasks SET exit_code = ?, error_message = ?, updated_at = ? WHERE id = ?",
		exitCode, errorMessage, time.Now(), id,
	)
	return err
}

func (db *DB) UpdateTaskError(id int64, errMsg string) error {
	_, err := db.Exec(
		"UPDATE tasks SET error_message = ?, status = ?, updated_at = ? WHERE id = ?",
		errMsg, task.StatusFailed, time.Now(), id,
	)
	return err
}

func (db *DB) UpdateTaskContext(id int64, taskContext string) error {
	_, err := db.Exec(
		"UPDATE tasks SET context = ?, updated_at = ? WHERE id = ?",
		taskContext, time.Now(), id,
	)
	return err
}

func (db *DB) FinalizeTaskIdentity(id int64, title, slug, branch string) error {
	_, err := db.Exec(
		"UPDATE tasks SET title = ?, slug = ?, branch = ?, updated_at = ? WHERE id = ?",
		title, slug, branch, time.Now(), id,
	)
	return err
}

func (db *DB) ResetTaskForRetry(id int64) error {
	_, err := db.Exec(
		`UPDATE tasks SET status = ?, step_index = 0, current_step = NULL,
		 exit_code = NULL, error_message = NULL, started_at = NULL,
		 completed_at = NULL, updated_at = ? WHERE id = ?`,
		task.StatusPending, time.Now(), id,
	)
	return err
}

func (db *DB) DeleteTask(id int64) error {
	_, err := db.Exec("DELETE FROM task_dependencies WHERE task_id = ? OR blocked_by = ?", id, id)
	if err != nil {
		return err
	}
	_, err = db.Exec("DELETE FROM tasks WHERE id = ?", id)
	return err
}

func scanTask(row *sql.Row) (*task.Task, error) {
	var t task.Task
	var projectID sql.NullInt64
	var title, slug, workflow, branch sql.NullString
	var currentStep, worktreePath, errorMessage sql.NullString
	var taskContext sql.NullString
	var exitCode sql.NullInt64
	var startedAt, completedAt sql.NullTime
	var updatedAt sql.NullTime

	err := row.Scan(
		&t.ID, &projectID, &title, &t.Description, &slug, &workflow, &t.Status,
		&t.StepIndex, &currentStep,
		&branch, &worktreePath, &exitCode, &errorMessage, &taskContext,
		&t.CreatedAt, &startedAt, &completedAt, &updatedAt,
	)
	if err != nil {
		return nil, err
	}

	if projectID.Valid {
		t.ProjectID = projectID.Int64
	}
	if title.Valid {
		t.Title = title.String
	}
	if slug.Valid {
		t.Slug = slug.String
	}
	if workflow.Valid {
		t.Workflow = workflow.String
	}
	if currentStep.Valid {
		t.CurrentStep = currentStep.String
	}
	if branch.Valid {
		t.Branch = branch.String
	}
	if worktreePath.Valid {
		t.WorktreePath = worktreePath.String
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		t.ExitCode = &code
	}
	if errorMessage.Valid {
		t.ErrorMessage = errorMessage.String
	}
	if taskContext.Valid {
		t.Context = taskContext.String
	}
	if startedAt.Valid {
		t.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	if updatedAt.Valid {
		t.UpdatedAt = updatedAt.Time
	}

	return &t, nil
}

func scanTasks(rows *sql.Rows) ([]*task.Task, error) {
	var tasks []*task.Task

	for rows.Next() {
		var t task.Task
		var projectID sql.NullInt64
		var title, slug, workflow, branch sql.NullString
		var currentStep, worktreePath, errorMessage sql.NullString
		var taskContext sql.NullString
		var exitCode sql.NullInt64
		var startedAt, completedAt sql.NullTime
		var updatedAt sql.NullTime

		err := rows.Scan(
			&t.ID, &projectID, &title, &t.Description, &slug, &workflow, &t.Status,
			&t.StepIndex, &currentStep,
			&branch, &worktreePath, &exitCode, &errorMessage, &taskContext,
			&t.CreatedAt, &startedAt, &completedAt, &updatedAt,
		)
		if err != nil {
			return nil, err
		}

		if projectID.Valid {
			t.ProjectID = projectID.Int64
		}
		if title.Valid {
			t.Title = title.String
		}
		if slug.Valid {
			t.Slug = slug.String
		}
		if workflow.Valid {
			t.Workflow = workflow.String
		}
		if currentStep.Valid {
			t.CurrentStep = currentStep.String
		}
		if branch.Valid {
			t.Branch = branch.String
		}
		if worktreePath.Valid {
			t.WorktreePath = worktreePath.String
		}
		if exitCode.Valid {
			code := int(exitCode.Int64)
			t.ExitCode = &code
		}
		if errorMessage.Valid {
			t.ErrorMessage = errorMessage.String
		}
		if taskContext.Valid {
			t.Context = taskContext.String
		}
		if startedAt.Valid {
			t.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}
		if updatedAt.Valid {
			t.UpdatedAt = updatedAt.Time
		}

		tasks = append(tasks, &t)
	}

	return tasks, rows.Err()
}
