# internal/db — SQLite Persistence

Schema, migrations, task/project queries. Load `/database` skill before making substantial changes.

## Critical Invariants

- **`ClaimTask(id)` is atomic: pending → running with `started_at`** — returns false if not pending; prevents duplicate execution
- **`GetClaimableTasks()` filters blocked dependencies, orders by priority desc then `created_at` asc** — ordering matters for fairness
- **Parameterized queries only (`?` placeholders)** — never use string interpolation in SQL
