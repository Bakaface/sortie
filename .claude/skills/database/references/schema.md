# Database Schema Reference

## Tables

### projects
```sql
id INTEGER PRIMARY KEY AUTOINCREMENT
path TEXT UNIQUE NOT NULL
name TEXT NOT NULL DEFAULT ''
default_priority TEXT NOT NULL DEFAULT 'medium'
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```

### tasks
```sql
id INTEGER PRIMARY KEY AUTOINCREMENT
project_id INTEGER REFERENCES projects(id)
title TEXT NOT NULL DEFAULT ''
description TEXT NOT NULL
slug TEXT NOT NULL DEFAULT ''
workflow TEXT NOT NULL DEFAULT ''
status TEXT NOT NULL DEFAULT 'pending'
priority TEXT NOT NULL DEFAULT 'medium'
step_index INTEGER NOT NULL DEFAULT 0
current_step TEXT NOT NULL DEFAULT ''
loop_iteration INTEGER NOT NULL DEFAULT 0
branch_name TEXT NOT NULL DEFAULT ''
branch TEXT NOT NULL DEFAULT ''
worktree BOOLEAN NOT NULL DEFAULT 1
worktree_path TEXT NOT NULL DEFAULT ''
exit_code INTEGER
error_message TEXT NOT NULL DEFAULT ''
context TEXT NOT NULL DEFAULT ''
images TEXT NOT NULL DEFAULT '[]'
created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
started_at DATETIME
completed_at DATETIME
updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
```

### task_dependencies
```sql
task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE
depends_on_task_id INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE
PRIMARY KEY (task_id, depends_on_task_id)
```

## Migration History

| Version | Change |
|---------|--------|
| v1 | Initial tasks table |
| v2 | Add workflow column |
| v3-v4 | Additional task fields |
| v5 | Projects table with FK, project_id on tasks |
| v6 | Task dependencies table |
| v7 | Priority support (low/medium/high/urgent) |
| v8 | loop_iteration for closed-loop workflows |
| v9 | branch_name for user-provided branch templates |
| v10 | worktree boolean for optional worktree isolation |

## Adding Migrations

Append to the `migrations` slice in `db.go`:

```go
{11, `ALTER TABLE tasks ADD COLUMN new_field TEXT NOT NULL DEFAULT ''`},
```

Auto-applied on startup via `ensureSchema()`.
