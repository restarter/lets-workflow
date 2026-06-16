# Dolt Remote Server

Shared Dolt SQL server on VPS for multi-project beads sync. Developers connect directly via MySQL protocol - no local dolt server needed, no merge conflicts.

## Architecture

```
Developer A (Linux)                                     Developer B (Mac)
     |                                                       |
 bd list / bd create                                    bd list / bd create
     |                                                       |
 MySQL protocol (3306)                               MySQL protocol (3306)
     |                                                       |
     +--------------------> VPS (Dolt SQL Server) <----------+
                                  |
                           /var/lib/dolt/data/
                           ├── lets/          # project 1
                           ├── aff/           # project 2
                           └── test/          # project 3
```

One dolt container serves all project databases. Each project has its own database. Users get per-database grants (not superuser). IP allowlist restricts access to known developer IPs.

## VPS Setup

### Initial deploy

```bash
ssh root@vps "bash -s -- --expose-sql lets aff test" < scripts/remote/dolt/setup-remote.sh
```

This will:
- Install Docker (if not present)
- Create dolt container with health check and resource limits
- Expose SQL port 3306 externally, restrict remotesapi to localhost
- Initialize databases: `lets`, `aff`, `test`
- Generate root password and save to `/opt/dolt-remote/.env`

### Pin dolt version

```bash
ssh root@vps "echo 'DOLT_VERSION=1.83.0' >> /opt/dolt-remote/.env"
```

Then restart: `ssh root@vps "cd /opt/dolt-remote && docker compose up -d"`

### Add SQL user

Use `manage-users.sh` (runs on the VPS; talks to the server via a throwaway MySQL client on `dolt-net`, so the user is visible to the server - no host client, no IP-allowlist involvement):

```bash
# Create a user scoped to specific databases (prints a generated password ONCE)
ssh root@vps "bash -s -- --add-user alice --dbs lets,aff" < scripts/remote/dolt/manage-users.sh

# Add more databases later (additive grant, password unchanged)
ssh root@vps "bash -s -- --grant alice --dbs new_project" < scripts/remote/dolt/manage-users.sh

# Set the EXACT database set (removes others) - DROP+CREATE, ROTATES the password
ssh root@vps "bash -s -- --set-dbs alice --dbs lets" < scripts/remote/dolt/manage-users.sh

# Inspect / remove
ssh root@vps "bash -s -- --list-users"          < scripts/remote/dolt/manage-users.sh
ssh root@vps "bash -s -- --list-grants alice"   < scripts/remote/dolt/manage-users.sh
ssh root@vps "bash -s -- --remove-user alice"   < scripts/remote/dolt/manage-users.sh
```

**Why the script (don't do it by hand):**
- Users created via `docker exec dolt dolt sql` are **invisible to the SQL server**. The script connects *through* the server (MySQL protocol) over `dolt-net`, so grants take effect.
- Dolt has **no `REVOKE`**: `--grant` is additive and keeps the password; **removing** a database from a user requires `--set-dbs` (DROP+CREATE), which **rotates** the password (printed once).

Grant access to specific databases only - never `GRANT ALL ON *.*`. The IP allowlist is the network layer; per-database grants are the authorization layer.

### Add database

```bash
ssh root@vps "bash -s -- --add-repo new-project" < scripts/remote/dolt/setup-remote.sh
```

## IP Allowlist

Access to SQL port 3306 is restricted by IP. Only allowed IPs can connect.

```bash
# Allow an IP (single IPv4 only - no CIDR/masks)
ssh root@vps "bash -s -- --allow-ip 1.2.3.4" < scripts/remote/dolt/setup-remote.sh

# Remove an IP
ssh root@vps "bash -s -- --remove-ip 1.2.3.4" < scripts/remote/dolt/setup-remote.sh

# Decommission the port entirely (drop all ALLOW + REJECT rules for the port)
ssh root@vps "bash -s -- --purge-port 3306" < scripts/remote/dolt/setup-remote.sh

# Check current rules
ssh root@vps "iptables -L DOCKER-USER -n --line-numbers | grep 3306"
```

Rules persist across reboots via `iptables-persistent`.

## Client Setup

### 1. Configure beads

The project's `.beads/metadata.json` contains shared config (committed to git):

```json
{
  "database": "dolt",
  "backend": "dolt",
  "dolt_mode": "server",
  "dolt_database": "<project-database>"
}
```

Server connection details are set via environment variables (not committed to git):

| Variable | Description | Example |
|----------|-------------|---------|
| `BEADS_DOLT_SERVER_HOST` | VPS IP address | `203.0.113.42` |
| `BEADS_DOLT_SERVER_PORT` | SQL port (default 3306) | `3306` |
| `BEADS_DOLT_SERVER_USER` | Your SQL username | `myuser` |
| `BEADS_DOLT_PASSWORD` | Your SQL password | `mypassword` |

### 2. Set connection environment variables

#### Linux / macOS (bash)

```bash
cat >> ~/.bashrc << 'EOF'
export BEADS_DOLT_SERVER_HOST=<vps-ip>
export BEADS_DOLT_SERVER_PORT=3306
export BEADS_DOLT_SERVER_USER=<username>
export BEADS_DOLT_PASSWORD=<password>
EOF
source ~/.bashrc
```

#### Linux / macOS (zsh)

```bash
cat >> ~/.zshrc << 'EOF'
export BEADS_DOLT_SERVER_HOST=<vps-ip>
export BEADS_DOLT_SERVER_PORT=3306
export BEADS_DOLT_SERVER_USER=<username>
export BEADS_DOLT_PASSWORD=<password>
EOF
source ~/.zshrc
```

#### Windows (PowerShell, run as Administrator)

```powershell
setx BEADS_DOLT_SERVER_HOST "<vps-ip>"
setx BEADS_DOLT_SERVER_PORT "3306"
setx BEADS_DOLT_SERVER_USER "<username>"
setx BEADS_DOLT_PASSWORD "<password>"
```

Restart the terminal after running `setx`.

### 3. Verify

```bash
bd list
bd show <any-task-id>
```

If connection works, you'll see tasks. No local dolt server needed.

## Onboarding New Developer

1. **Get access:** Admin adds developer's IP to the allowlist (`--allow-ip`)
2. **Get credentials:** Admin creates SQL user with per-database grants (see Add SQL user)
3. **Configure:** Verify `.beads/metadata.json` has `dolt_database` set (should be committed in repo)
4. **Set env vars:** Add `BEADS_DOLT_SERVER_HOST`, `BEADS_DOLT_SERVER_PORT`, `BEADS_DOLT_SERVER_USER`, `BEADS_DOLT_PASSWORD` to shell profile (see Client Setup)
5. **Verify:** Run `bd list` - should show project tasks

## Project Identity Mismatch (cross-checkout safety)

`bd` refuses to connect to a Dolt database whose `project_id` differs from the local checkout's. You'll see:

```
PROJECT IDENTITY MISMATCH — refusing to connect
  Local project ID (metadata.json):  608faa47-...
  Database project ID:               8322ceda-...
```

This is intentional safety: it prevents accidental task pollution between unrelated projects that share the same VPS. Each project has a unique `project_id` stored in `.beads/metadata.json` (committed to the repo). The Dolt server stores its own `project_id` next to the database; `bd` compares them on connect.

**To use the same database from a sibling checkout** (e.g. a worktree, a fresh clone, or a second machine): copy `.beads/metadata.json` (or just the `project_id` field) from the source checkout into the new checkout's `.beads/metadata.json` before running `bd`. Server data is shared; identity is per-checkout.

If you intentionally want a fresh project against the same VPS, generate a new `project_id` (e.g. `uuidgen`) and use a separate `dolt_database` name in `.beads/metadata.json`.

## Adding New Project

1. Add database on VPS:
   ```bash
   ssh root@vps "bash -s -- --add-repo my-project" < scripts/remote/dolt/setup-remote.sh
   ```
2. Grant access to existing users:
   ```bash
   mysql -h <vps-ip> -u root -p<root-password> -e "
     GRANT ALL ON my_project.* TO 'username'@'%';
     FLUSH PRIVILEGES;
   "
   ```
3. In the new project, run `bd init` to create `.beads/` directory
4. Verify `.beads/metadata.json` has `"dolt_database": "my-project"`
5. Set env vars if not already in shell profile (see Client Setup)
6. Run `bd list` to verify

## Backups

Two layers:

1. **VPS-level snapshots** (host provides daily + weekly snapshots of whole VPS) - the disaster-recovery layer. Restore the entire VPS state.
2. **Application-aware snapshot** via `scripts/remote/dolt/backup-remote.sh` - operator-initiated, point-in-time snapshot of just `/opt/dolt-remote/` for **before-risky-op** scenarios (migrations, version bumps, ad-hoc DDL). Single tarball, fast restore without rolling back the whole VPS.

This section documents layer 2.

### When to use

Run before any operation that would be hard to roll back via VPS-snap alone:
- `setup-remote.sh` re-run with config changes
- `DOLT_VERSION` bump
- Manual SQL migrations or schema changes
- Bulk `bd` operations that affect many issues at once

### Run a backup

```bash
ssh root@vps "bash -s" < scripts/remote/dolt/backup-remote.sh
```

The container stops for ~5-15 seconds. Full state lands in `/opt/dolt-remote/backups/dolt-backup-YYYYMMDD-HHMMSS.tar.gz` (chmod 600 - contains `.env` with `ROOT_PASSWORD`).

Custom output dir:
```bash
ssh root@vps "bash -s -- --output-dir /mnt/backups" < scripts/remote/dolt/backup-remote.sh
```

### Verify a backup

```bash
ssh root@vps "gzip -t /opt/dolt-remote/backups/dolt-backup-*.tar.gz && \
              tar -tzf /opt/dolt-remote/backups/dolt-backup-YYYYMMDD-HHMMSS.tar.gz | head"
```

Expected: no errors from `gzip -t`; tar listing shows `dolt-home/`, `.env`, `docker-compose.yml`, `servercfg.d/`.

### Restore from backup

The tarball contains the entire deployment. Restore replaces the install dir's contents in-place (preserves only `backups/`):

```bash
ssh root@vps "
  set -e
  cd /opt/dolt-remote
  docker compose down
  find . -mindepth 1 -maxdepth 1 ! -name backups -exec rm -rf {} +
  tar -xzf backups/dolt-backup-YYYYMMDD-HHMMSS.tar.gz
  chmod 600 .env
  docker compose up -d
"
```

Verify:
```bash
ssh root@vps "docker exec dolt dolt sql -q 'SHOW DATABASES;'"
```

> **Note:** dependent services (e.g., `beads-web`) keep running but may briefly fail until Dolt finishes booting (~5-10s). The `dolt-net` Docker network is preserved.

### Manage backups

There is no automatic retention. Delete tarballs you no longer want:

```bash
ssh root@vps "ls -lh /opt/dolt-remote/backups/"
ssh root@vps "rm /opt/dolt-remote/backups/dolt-backup-YYYYMMDD-HHMMSS.tar.gz"
```

Long-term retention is covered by the VPS-level snapshot system; this script is for short-lived "before-risky-op" snapshots only.

### Troubleshooting

If the backup script exits with `dolt did not reach healthy state in 60s`: the tarball is still valid (chmod 600, gzip-tested). Inspect `docker logs dolt` before relying on the running container; the backup itself is safe to keep.

## Server Layout

```
/opt/dolt-remote/
├── .env                  # ROOT_PASSWORD, DOLT_VERSION (chmod 600)
├── docker-compose.yml    # Container config
├── servercfg.d/
│   └── config.yaml       # Dolt server config
└── dolt-home/            # Mounted as /var/lib/dolt
    ├── .doltcfg/         # Privileges, global config
    └── data/
        ├── lets/         # Database: lets
        ├── aff/          # Database: aff
        └── test/         # Database: test
```

## Docker Network

Dolt container joins an external `dolt-net` network (created automatically by setup script). Other services (e.g. beads-ui) connect to Dolt through this shared network. The network is not managed by any single docker-compose - it persists independently.

## Ports

| Port | Binding | Purpose |
|------|---------|---------|
| 3306 | all interfaces | Direct SQL (MySQL protocol) - primary |
| 50051 | localhost only | remotesapi (legacy push/pull) |

## Security

- **IP allowlist** via DOCKER-USER iptables chain - first line of defense
- **SQL accounts** - per-user authentication
- **Password** via environment variable (`BEADS_DOLT_PASSWORD`) - not stored in git
- **remotesapi** restricted to localhost when Direct SQL is enabled
- **No TLS** - acceptable for trusted developer network with IP allowlist. For additional encryption, use SSH tunneling:
  ```bash
  ssh -L 3306:localhost:3306 root@vps
  # Then connect to localhost:3306 instead of vps-ip:3306
  ```
  For full TLS: needs a domain (Let's Encrypt) or reverse proxy

## Troubleshooting

### "No authentication methods available for authentication"

User was created via `docker exec dolt dolt sql` instead of through SQL protocol. Recreate the user by connecting as root via MySQL:

```bash
mysql -h <vps-ip> -u root -p<root-password> -e "
  CREATE USER IF NOT EXISTS 'username'@'%' IDENTIFIED BY 'password';
  GRANT ALL ON lets.* TO 'username'@'%';
  FLUSH PRIVILEGES;
"
```

### Changing user grants (REVOKE not supported)

Dolt does not support `REVOKE ALL PRIVILEGES ON *.* FROM ...`. To change grants, drop and recreate the user:

```bash
mysql -h <vps-ip> -u root -p<root-password> -e "
  DROP USER 'username'@'%';
  CREATE USER 'username'@'%' IDENTIFIED BY 'password';
  GRANT ALL ON lets.* TO 'username'@'%';
  GRANT ALL ON aff.* TO 'username'@'%';
  FLUSH PRIVILEGES;
"
```

### "Access denied for user"

Wrong password. Verify `BEADS_DOLT_PASSWORD` is set:

```bash
echo $BEADS_DOLT_PASSWORD
```

### "connection refused" or timeout

1. Check if your IP is in the allowlist:
   ```bash
   ssh root@vps "iptables -L DOCKER-USER -n | grep 3306"
   ```
2. Check container is running:
   ```bash
   ssh root@vps "docker ps | grep dolt"
   ```
3. Check container health:
   ```bash
   ssh root@vps "docker inspect dolt --format='{{.State.Health.Status}}'"
   ```

### "table not found" on VPS with existing data

Data was imported via `dolt sql` CLI inside the container, which uses a different storage path than the SQL server (`data_dir`). Re-import through the SQL protocol:

```bash
# Dump from CLI storage
ssh root@vps "docker exec dolt dolt sql -q 'USE <db>; SELECT 1;'"

# Import via MySQL protocol
mysql -h <vps-ip> -u root -p<root-password> <db> < dump.sql
```

### Container not starting

```bash
ssh root@vps "cd /opt/dolt-remote && docker compose logs --tail 50"
```

### Reset everything

```bash
ssh root@vps "cd /opt/dolt-remote && docker compose down -v && rm -rf dolt-home && rm .env"
# Then re-run the script
```

## Legacy: RemotesAPI (push/pull)

The older push/pull architecture is still available via remotesapi on port 50051 (localhost only when `--expose-sql` is used). See `scripts/deprecated/beads/setup-beads-remote.sh` for the client-side setup (now archived under `scripts/deprecated/`). This mode requires a local dolt server and is no longer recommended.