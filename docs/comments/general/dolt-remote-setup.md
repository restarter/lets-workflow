# Beads Remote Sync via GitHub

Shared beads database synced through a GitHub repo. Each developer works locally, syncs via push/pull.

```
Developer A                          Developer B
.beads/dolt/ (local)                 .beads/dolt/ (local)
     |                                    |
     +--- bd dolt push --->  GitHub  <--- bd dolt pull ---+
                          (refs/dolt/data)
```

No VPS, no Docker, no SQL server. Just a private GitHub repo as remote storage.

## How It Works

- Dolt v1.81.10+ can use any git repo as a remote
- Data stored on a special ref `refs/dolt/data` (invisible in GitHub UI, doesn't affect normal branches)
- Auth via your existing git/gh credentials (HTTPS) or SSH keys
- Each developer has a full local copy, syncs with push/pull - like git

## Initial Setup (one-time, project admin)

### 1. Create a private GitHub repo for beads data

```bash
gh repo create myorg/myproject-beads --private --description "Beads issue database"
# Init with a README (required - dolt can't push to empty repo)
cd /tmp && git clone https://github.com/myorg/myproject-beads.git
cd myproject-beads && echo "# Beads data" > README.md && git add . && git commit -m "init" && git push
cd - && rm -rf /tmp/myproject-beads
```

### 2. Init beads in your project

```bash
cd myproject

# Init dolt database first
mkdir -p .beads/dolt/myproj
cd .beads/dolt/myproj
dolt init --name "beads" --email "beads@local"
cd -

# Start dolt server, then run bd init
cd .beads/dolt/myproj && dolt sql-server --port 3307 --host 127.0.0.1 & cd -
sleep 2
bd init --prefix myproj
kill %1
```

### 3. Connect to GitHub remote

```bash
# Add remote via dolt directly (bd dolt remote add has auto-push issues)
cd .beads/dolt/myproj
dolt remote add origin https://github.com/myorg/myproject-beads.git
cd -

# First push needs --force (empty remote, no common ancestor)
bd dolt push --force
```

### 4. Verify

```bash
gh api repos/myorg/myproject-beads/git/refs --jq '.[].ref'
# Should show: refs/dolt/data
```

### 5. Commit and push your project

The project repo (not beads repo) should have these tracked files:
- `.beads/config.yaml` - shared config
- `.beads/metadata.json` - backend config
- `.beads/.gitignore` - ignores local dolt data

## Onboarding New Developer

After cloning the project:

```bash
scripts/beads/setup-beads-remote.sh
```

That's it. The script handles everything:
1. Inits dolt database + beads schema (dolt init, dolt sql-server, bd init)
2. Detects git auth method (SSH vs HTTPS) and connects to the GitHub remote
3. Fetches and syncs the remote database

### What the script does (if you prefer manual steps)

```bash
# 1. Init dolt database + beads schema
rm -f .beads/metadata.json    # clean slate
rm -rf .beads/dolt
mkdir -p .beads/dolt/lets
cd .beads/dolt/lets
dolt init --name "beads" --email "beads@local"
cd -

# 2. Start dolt server, run bd init
cd .beads/dolt/lets
dolt sql-server --port 3307 --host 127.0.0.1 &
cd -
sleep 2
bd init --force --prefix lets
kill %1  # stop dolt server

# 3. Add remote (use SSH or HTTPS depending on your auth)
cd .beads/dolt/lets           # prefix = database directory name
# SSH:   dolt remote add origin git@github.com:restarter/lets-workflow-beads.git
# HTTPS: dolt remote add origin https://github.com/restarter/lets-workflow-beads.git
dolt fetch origin
dolt reset --hard origin/main
cd -

# 4. Verify (needs dolt server running)
bd list
```

## Daily Workflow

```bash
bd dolt pull          # pull latest from team
# ... work normally with bd create, bd update, bd close ...
bd dolt push          # push your changes
```

Push/pull happen automatically in some bd operations (e.g. `bd dolt remote add` triggers auto-push). For explicit sync, use the commands above.

## Project Files

```
.beads/
├── config.yaml      # Tracked: shared settings
├── metadata.json    # Tracked: backend=dolt, prefix
├── .gitignore       # Tracked: ignores dolt/, runtime files
├── dolt/            # Gitignored: local dolt database
│   └── lets/        #   database with all issues
│       └── .dolt/   #   dolt internals + remote config
└── ...
```
