---
name: skill-audit
description: >
  Audit domain skills against the current codebase to find drift, gaps, and stale claims,
  then automatically apply fixes. Use when the user says 'audit skills', 'check skills',
  'are skills up to date', 'skill drift', 'review domain skills', 'which skills need updating',
  or wants to verify that domain skill content (file maps, structs, patterns, conventions)
  still matches reality. Performs comprehensive codebase research per skill, produces a
  structured delta report, and applies updates to drifted skills.
allowed-tools: Bash, Read, Write, Edit, Glob, Grep, Agent
---

# Skill Audit

Audit domain skills against the live codebase to detect drift and gaps, then fix them.

## Input

Optional: a list of specific skills to audit. If none provided, audit all domain skills from the CLAUDE.md table.

## Workflow

### Phase 1: Audit

#### 1. Parse the Domain Skills Registry

Read the project CLAUDE.md and extract the "Domain Skills" table mapping package paths to skill names.

#### 2. For Each Domain Skill

Spawn an Explore agent (model: opus) per skill to verify claims against reality. Each agent should:

1. **Read the skill's SKILL.md** (and any `references/` files)
2. **Extract verifiable claims**: file map entries, struct definitions, key patterns, adjacent packages
3. **Verify each claim against the codebase**:
   - Do listed files still exist at stated paths?
   - Do struct definitions match current code?
   - Are there new files in the package not mentioned in the file map?
   - Have patterns or conventions changed?
   - Are adjacent package relationships still accurate?
4. **Return a structured delta** (see report format below)

#### 3. Identify Uncovered Packages

Glob for `internal/*/` and `cmd/*/` directories. Flag any package directory that has no corresponding skill in the registry table.

#### 4. Compile Report

Aggregate per-skill deltas into a single report. Present to the user.

### Phase 2: Apply Fixes

After the report is compiled, automatically update all drifted skills:

#### 5. For Each Drifted Skill

Spawn an implementer agent per skill that needs updating. Each agent receives:
- The skill's SKILL.md path (and any references/ files)
- The package path(s) it covers
- The specific audit findings (delta report for that skill)

Each agent should:

1. **Read the current SKILL.md** and any `references/` files
2. **Read the actual source files** in the package to get accurate current state
3. **Apply all fixes** from the audit findings:
   - Update file map tables (add unlisted files, remove missing files)
   - Update struct definitions to match current code
   - Fix function signatures
   - Add/remove/correct pattern descriptions
   - Update references/ files if they exist
4. **Preserve the skill's existing structure and style** — only change what's drifted
5. **Do NOT add test files** to file maps (they are intentionally omitted)

#### 6. Summary

Report which skills were updated and a brief summary of changes per skill.

## Report Format

```markdown
# Skill Audit Report

**Date**: YYYY-MM-DD
**Skills audited**: N

## Summary

- X skills up to date
- Y skills need updates
- Z uncovered packages

## Per-Skill Results

### /skill-name — STATUS (up-to-date | needs-update | stale)

**Package**: `internal/foo/`

#### File Map Delta
| File | Status | Detail |
|------|--------|--------|
| `existing.go` | ok | — |
| `removed.go` | missing | Listed in skill but not on disk |
| `new_file.go` | unlisted | Exists on disk but not in skill |

#### Struct Drift
- `FooConfig`: field `NewField` added (not in skill)

#### Pattern Changes
- Skill says X, code now does Y

#### Recommendation
Brief description of what to update.

---

## Uncovered Packages

| Package | Files | Suggested Action |
|---------|-------|-----------------|
| `internal/notify/` | 3 | Consider creating a skill |
```

## Guidelines

- Use `model: "opus"` for Explore agents in Phase 1 — accuracy matters more than speed
- Use implementer agents in Phase 2 — they have Edit/Write tools for applying fixes
- Run both phases in parallel (one agent per skill) for throughput
- Focus on verifiable, structural claims — skip subjective "patterns" that require deep interpretation
- A skill is "up-to-date" if no file map mismatches and no struct drift
- A skill is "needs-update" if minor drift (new files, small struct changes)
- A skill is "stale" if major structural changes (deleted files, renamed packages, rewritten modules)
- When applying fixes, read the actual source code — don't rely solely on audit findings for exact syntax
- Preserve existing skill style and formatting conventions
- Do NOT add test files to file maps — they are intentionally omitted from skills
