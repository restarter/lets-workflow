# Dolt Remote for Beads - Setup Guide

Shared beads database over a remote Dolt SQL server. Team members connect directly - no push/pull, no sync.

```
Developer A  --->  VPS: Dolt SQL :3306  <---  Developer B
(config.yaml)      /opt/dolt-remote/          (config.yaml)
(.env creds)       └── dolt-home/data/        (.env creds)
                       └── project/
```

## 1. Deploy Server (one-time)

Need a VPS with root SSH access. Script handles Docker, Dolt, everything.

```bash
# Install + create repo + expose SQL port
ssh root@VPS "bash -s -- --expose-sql my-project" < scripts/dolt/setup-remote.sh

# Create accounts for the team
ssh root@VPS "bash -s -- --add-user alice:$(openssl rand -base64 12)" < scripts/dolt/setup-remote.sh
ssh root@VPS "bash -s -- --add-user bob:$(openssl rand -base64 12)" < scripts/dolt/setup-remote.sh
```

`--expose-sql` opens port 3306 externally (default is localhost-only). Root password auto-generated, saved to `/opt/dolt-remote/.env`.

Multiple repos at once:
```bash
ssh root@VPS "bash -s -- --expose-sql project-a project-b" < scripts/dolt/setup-remote.sh
```

## 2. Configure Project

### Init beads

```bash
bd init --prefix myproj
```

### Shared config (`.beads/config.yaml`, tracked in git)

Add at the end:

```yaml
dolt:
  server_mode: true
  server_host: "YOUR_VPS_IP"
  server_port: 3306
```

### Per-developer credentials (`.beads/.env`, gitignored)

```bash
BEADS_DOLT_SERVER_USER=alice
BEADS_DOLT_PASSWORD=secret123
```

`.env` is already in `.beads/.gitignore`. Create a template for the team:

```bash
# .beads/.env.example (tracked)
# Copy to .env and fill in your values:
#   cp .env.example .env
BEADS_DOLT_SERVER_USER=
BEADS_DOLT_PASSWORD=
```

### Test

```bash
bd list
bd create --title="Test" --type=task --priority=4
bd list
bd close <id>
```

## 3. Onboarding New Developer

1. Clone the repo (config.yaml already there)
2. `cp .beads/.env.example .beads/.env`
3. Fill in username and password from admin
4. `bd list` - done

No dolt install needed. No local database.

## 4. Day-to-Day VPS Management

```bash
# Add repo
ssh root@VPS "bash -s -- --add-repo new-project" < scripts/dolt/setup-remote.sh

# Add user
ssh root@VPS "bash -s -- --add-user newdev:pass123" < scripts/dolt/setup-remote.sh

# Check status
ssh root@VPS "cd /opt/dolt-remote && docker compose ps"

# Logs
ssh root@VPS "cd /opt/dolt-remote && docker compose logs --tail 20"

# Restart
ssh root@VPS "cd /opt/dolt-remote && docker compose restart"
```

## 5. What's on the Server

```
/opt/dolt-remote/
├── .env                  # ROOT_PASSWORD, ports
├── docker-compose.yml
├── servercfg.d/
│   └── config.yaml       # Dolt listener, remotesapi, data_dir
└── dolt-home/            # Mounted as /var/lib/dolt
    ├── .doltcfg/
    │   └── privileges.db # User accounts
    └── data/
        └── my-project/   # Repo data
```

## 6. What's in the Project

```
.beads/
├── config.yaml      # Tracked: dolt server_mode, host, port
├── .env             # Gitignored: user + password
├── .env.example     # Tracked: template for team
├── .gitignore       # Tracked: ignores .env, runtime files
└── metadata.json    # Tracked: backend=dolt, dolt_database=myproj
```
