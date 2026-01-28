CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    title TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL,
    slug TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    step_index INTEGER NOT NULL DEFAULT 0,
    current_step TEXT,
    branch TEXT NOT NULL DEFAULT '',
    worktree_path TEXT,
    exit_code INTEGER,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    completed_at DATETIME,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);

CREATE TABLE IF NOT EXISTS task_dependencies (
    task_id INTEGER NOT NULL REFERENCES tasks(id),
    blocked_by INTEGER NOT NULL REFERENCES tasks(id),
    PRIMARY KEY (task_id, blocked_by)
);
