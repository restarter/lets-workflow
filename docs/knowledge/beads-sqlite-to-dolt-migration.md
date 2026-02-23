# Beads: SQLite to Dolt Migration Guide

After upgrading beads CLI from v0.49 to v0.55+, the default backend changed from SQLite to Dolt. Existing SQLite databases are not auto-migrated - all `bd` commands fail with:

```
Error: Dolt backend configured but database not found
```

## Official Migration (recommended)

Since v0.55, beads has a built-in migration command that preserves issues, labels, dependencies, and config. This is the preferred method.

### Step 1: Remove stale Dolt directory (if exists)

If a previous failed migration or `bd doctor --fix` created an empty Dolt directory:

```bash
rm -rf .beads/dolt
```

Without this, `bd migrate` will refuse to run:
```
Error: Dolt directory already exists at .beads/dolt
```

### Step 2: Dry-run

```bash
bd migrate --to-dolt --dry-run
```

This shows what will be migrated: issue count, events, config keys. Verify the numbers match expectations.

### Step 3: Run migration

```bash
bd migrate --to-dolt --yes
```

What it does:
1. Creates backup of SQLite database (`beads.backup-pre-dolt-*.db`)
2. Creates new Dolt database at `.beads/dolt/`
3. Imports all issues with labels and dependencies
4. Imports events (history/comments)
5. Copies config values
6. Updates `metadata.json` and `config.yaml`

### Step 4: Verify

```bash
bd stats                # Issue counts match?
bd blocked              # Dependencies preserved?
bd ready                # Ready tasks visible?
bd epic status          # Parent-child relationships intact?
```

### Step 5: Cleanup and commit

```bash
# Remove old SQLite files
rm -f .beads/beads.db .beads/beads.db-shm .beads/beads.db-wal
rm -f .beads/beads.backup-pre-dolt-*.db

# Sync and commit
bd sync --flush-only
git add .beads/
git commit -m "chore: Migrate beads from SQLite to Dolt backend"
```

---

## Manual Migration (fallback)

Use this only if `bd migrate --to-dolt` fails or if you're on a version that doesn't have it.

### Prerequisites

- Beads v0.55+ installed (`brew upgrade beads`)
- Existing project with `.beads/` directory and SQLite backend
- JSONL file (`.beads/issues.jsonl`) is the migration bridge

### Step 1: Ensure JSONL is up to date

Before upgrading the CLI, make sure your JSONL export is current. If you already upgraded and `bd` commands fail - check if JSONL has your data:

```bash
wc -l .beads/issues.jsonl           # total records
grep -c '"status":"open"' .beads/issues.jsonl
grep -c '"status":"closed"' .beads/issues.jsonl
grep -c '"status":"tombstone"' .beads/issues.jsonl
```

If JSONL is empty but SQLite has data, you need the old CLI version to export:

```bash
# Temporary rollback to export
curl -fsSL "https://github.com/steveyegge/beads/releases/download/v0.49.6/beads_0.49.6_darwin_arm64.tar.gz" -o /tmp/beads_old.tar.gz
tar -xzf /tmp/beads_old.tar.gz -C /tmp
/tmp/bd sync --flush-only
```

### Step 2: Clean JSONL (optional but recommended)

Tombstone records (deleted issues) cause import errors in the new version:

```bash
# Backup original
cp .beads/issues.jsonl /tmp/beads-backup.jsonl

# Remove tombstones
grep -v '"status":"tombstone"' .beads/issues.jsonl > /tmp/beads-clean.jsonl
cp /tmp/beads-clean.jsonl .beads/issues.jsonl
```

Verify no broken references remain (tombstone IDs referenced by live issues):

```python
python3 -c "
import json

with open('.beads/issues.jsonl') as f:
    issues = [json.loads(l) for l in f]

with open('/tmp/beads-backup.jsonl') as f:
    tombstone_ids = {json.loads(l)['id'] for l in f if '\"tombstone\"' in l}

live_ids = {i['id'] for i in issues}

for issue in issues:
    for dep in (issue.get('depends_on') or []) + (issue.get('blocks') or []):
        ref = dep if isinstance(dep, str) else dep.get('depends_on_id', dep.get('issue_id', ''))
        if ref in tombstone_ids:
            print(f'BROKEN REF: {issue[\"id\"]} -> {ref}')
    parent = issue.get('parent', '')
    if parent in tombstone_ids:
        print(f'BROKEN PARENT: {issue[\"id\"]} -> {parent}')
"
```

If broken references exist - fix them in the JSONL before importing, or clean up after import with `bd dep remove`.

### Step 3: Remove old databases

```bash
rm -f .beads/beads.db .beads/beads.db-shm .beads/beads.db-wal
rm -rf .beads/dolt
```

### Step 4: Initialize Dolt and import

```bash
bd init --from-jsonl --prefix <your-prefix> --skip-hooks --force -q
bd import -i .beads/issues.jsonl
```

Verify:

```bash
bd stats
bd list --status=open
```

### Step 5: Restore relationships (if lost)

The manual import may not preserve parent-child and dependency relationships. Check:

```bash
bd epic status          # Should show children
bd blocked              # Should show real blockers
bd show <epic-id>       # Should list CHILDREN section
```

If relationships are missing, restore manually:

```bash
# Parent-child (epic -> task)
bd update <task-id> --parent=<epic-id>

# Dependencies (task depends on another)
bd dep add <task-id> <depends-on-id>
```

### Step 6: Sync and commit

```bash
bd sync --flush-only
bd doctor               # Verify health
git add .beads/
git commit -m "chore: Migrate beads from SQLite to Dolt backend"
```

---

## Known Issues

- **`bd migrate --to-dolt` needs clean slate** - remove `.beads/dolt/` if a previous attempt left an empty directory.
- **`bd init --from-jsonl` creates empty DB** - the flag doesn't actually import. Use `bd import -i` after init (manual method only).
- **Tombstones block manual import** - `bd import` fails on `"status":"tombstone"` records. Filter them out first.
- **Manual import loses relationships** - parent-child and dependency metadata may be lost. Always verify with `bd epic status` and `bd blocked`. Official `bd migrate --to-dolt` preserves them.
- **GitHub issue tracking this**: [steveyegge/beads#2016](https://github.com/steveyegge/beads/issues/2016)

## Anti-pattern: Epic as Blocker

While migrating, check for epic-as-blocker pattern - where child tasks have `depends_on` pointing to their parent epic. Epics should be containers, not blockers. Remove with:

```bash
bd dep remove <child-task> <parent-epic>
```
