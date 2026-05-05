# beads-web - Kanban Board

Kanban board for beads issue tracker. Rust (Axum) binary with embedded Next.js frontend, runs in Docker, connects to Dolt SQL server.

Source: [Shybko/beads-web](https://github.com/Shybko/beads-web) (fork of weselow/beads-web)

## Architecture

```
Developer browser
     |
http://VPS_IP:3008
     |
iptables DOCKER-USER (IP allowlist)
     |
[beads-web container]  --dolt-net-->  [Dolt container]
   Debian trixie-slim                  MySQL :3306
   Rust binary (beads-web)
     |
   Volumes:
   ./data/.beads/        -> /app/.beads          (metadata.json)
   ./data/kanban-ui/     -> ~/.local/share/kanban-ui  (projects SQLite DB)
```

## Scripts

| Script | Purpose |
|--------|---------|
| `setup-remote.sh` | Full install - Docker image, compose, firewall, everything |
| `update-remote.sh` | Quick update - rebuild image with latest binary, restart |

## Prerequisites

- Dolt server running in Docker (see `scripts/dolt/`)
- `dolt-net` Docker network (created automatically by setup script)
- SQL user with grants on target database

## Quick Start

### 1. Create SQL user in Dolt (if not exists)

See [scripts/dolt/README.md - Add SQL user](../dolt/README.md#add-sql-user).

### 2. Deploy beads-web

```bash
ssh root@vps "bash -s -- \
  --sql-user bdweb --sql-password your-password \
  --database lets --port 3008 \
  --allow-ip YOUR_IP_1 --allow-ip YOUR_IP_2" \
  < scripts/beads-web/setup-remote.sh
```

### 3. Add projects

beads-web supports multiple Dolt databases as separate projects. After deploy, add them via API:

```bash
# Add a project (from VPS)
docker exec beads-web curl -sf -X POST http://localhost:3008/api/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-project","path":"dolt://mydb"}'

# Add another project
docker exec beads-web curl -sf -X POST http://localhost:3008/api/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"another-project","path":"dolt://anotherdb"}'
```

The `dolt://` prefix tells beads-web to read/write via Dolt SQL. Each database on the Dolt server can be a separate project.

> **Note:** The UI "Add Project" button currently only supports local paths. Remote Dolt databases must be added via API. See [Shybko/beads-web#3](https://github.com/Shybko/beads-web/issues/3) for UI support progress.

### 4. Open in browser

```
http://VPS_IP:3008
```

## Updating

When the fork has new changes (binary re-uploaded to the same release tag):

```bash
ssh root@vps "bash -s" < scripts/beads-web/update-remote.sh
```

This rebuilds the Docker image with `--no-cache` (re-downloads the binary) and restarts the container. Project configuration (SQLite DB) is preserved via volume mount.

### Update workflow

1. Push changes to the fork (`Shybko/beads-web`)
2. Create a new release (via GitHub UI or GitHub Actions if configured)
3. Update VPS:
   ```bash
   ssh root@vps "bash -s" < scripts/beads-web/update-remote.sh
   ```

With `--version latest` (default), the script always downloads from the latest release. No need to specify version numbers.

### Version upgrade

To upgrade to a new version (new release tag):

```bash
ssh root@vps "bash -s -- \
  --sql-user bdweb --sql-password your-password \
  --database lets --port 3008 \
  --version 1.0.0" < scripts/beads-web/setup-remote.sh
```

The setup script is idempotent - it regenerates config and rebuilds the container.

## Migrating Port

If you have an existing deploy on a different port (the historical default was 9090; the current default is 3008) and want to switch:

> **Migrating from beads-ui (port 9080) instead?** beads-ui is deprecated and lives at `/opt/beads-ui/` - different install path, different cleanup. See [scripts/deprecated/beads-ui/README.md](../deprecated/beads-ui/README.md) for decommission steps before deploying beads-web.

> **Order matters:** redeploy on the new port FIRST, then purge old-port firewall rules. The reverse order would leave the old container running on the old port without any firewall (open to internet) for the few seconds between purge and rebuild.

```bash
# 0. Pre-flight: confirm the new port is not already in use on the host.
ssh root@vps "ss -tlnp | grep -q ':3008' && echo BUSY || echo '3008 free'"
# Must show "3008 free" before continuing.

# 1. Redeploy on the new port. Sources existing /opt/beads-web/.env so SQL
#    credentials don't have to be retyped. Rebuild downs the old container,
#    so the old port stops listening as a side effect.
ssh root@vps 'set -a; source /opt/beads-web/.env; set +a; \
  bash -s -- \
    --sql-user "$SQL_USER" --sql-password "$SQL_PASSWORD" \
    --database "$DATABASE" --port 3008 \
    --allow-ip YOUR_IP_1 --allow-ip YOUR_IP_2' \
  < scripts/beads-web/setup-remote.sh

# 2. Verify new deployment BEFORE purging old port. If this fails, do NOT
#    proceed - the rollback path needs the old firewall rules intact.
ssh root@vps "
  docker ps --filter name=beads-web --format '{{.Status}}'
  curl -sf -o /dev/null -w 'HTTP: %{http_code}\n' http://localhost:3008/
"
# Must show "Up (healthy)" and "HTTP: 200".

# 3. Purge old-port firewall rules (now that new deployment is verified).
ssh root@vps "bash -s -- --purge-port 9090" \
  < scripts/beads-web/setup-remote.sh
```

Step 1 regenerates `docker-compose.yml`, `Dockerfile`, `.env`, and rebuilds the container. The volume-mounted `data/` (projects SQLite + Dolt metadata) is preserved.

Final verification:

```bash
ssh root@vps "ss -tlnp 2>/dev/null | grep ':9090' || echo 'OK: 9090 not listening'"
ssh root@vps "iptables -L DOCKER-USER -n | grep 9090 || echo 'OK: 9090 firewall clean'"
```

**Rollback:** if Step 1 or 2 fails, redeploy on the old port with the original arguments (`--port 9090 --allow-ip YOUR_IP`). Skip Step 3. The install dir and data volume are preserved across both directions; the old port's firewall rules are still in place because we deferred the purge.

> **Expected downtime:** Step 1 runs `docker compose build --no-cache` which re-downloads the binary from GitHub Releases. Total downtime is typically 30-90 seconds (build) + ~5-10 seconds (container recreate). Plan migrations outside business hours if uptime matters.

## IP Allowlist

```bash
# Add IP
ssh root@vps "bash -s -- --allow-ip 1.2.3.4 --port 3008" \
  < scripts/beads-web/setup-remote.sh

# Remove IP
ssh root@vps "bash -s -- --remove-ip 1.2.3.4 --port 3008" \
  < scripts/beads-web/setup-remote.sh
```

Multiple IPs can be added in a single command:

```bash
ssh root@vps "bash -s -- \
  --allow-ip 1.2.3.4 --allow-ip 5.6.7.8 --allow-ip 9.10.11.12 \
  --port 3008" < scripts/beads-web/setup-remote.sh
```

## Fork Support

By default, the binary is downloaded from `Shybko/beads-web`. Use `--repo` to specify a different fork:

```bash
ssh root@vps "bash -s -- \
  --sql-user bdweb --sql-password your-password \
  --database lets --port 3008 \
  --repo weselow/beads-web --version 1.0.0 \
  --allow-ip YOUR_IP" < scripts/beads-web/setup-remote.sh
```

## Server Layout

```
/opt/beads-web/                # Default install dir
├── .env                       # Credentials (chmod 600)
├── docker-compose.yml
├── Dockerfile
└── data/
    ├── .beads/
    │   └── metadata.json      # Dolt database config
    └── kanban-ui/
        └── settings.db        # Projects & tags (SQLite, persisted via volume)
```

## CLI Reference

### setup-remote.sh

**Full install** (all required):

| Argument | Default | Description |
|----------|---------|-------------|
| --sql-user | - | SQL username for Dolt connection |
| --sql-password | - | SQL password |
| --database | - | Dolt database name (e.g., lets, aff) |
| --allow-ip | - | Restrict access to this IP (single IPv4, no CIDR; repeatable for multiple IPs) |
| --no-firewall | - | Leave port open to all (NOT recommended) |
| --port | 3008 | Web UI port |
| --install-dir | /opt/beads-web | Installation directory |
| --dolt-host | dolt | Dolt container hostname |
| --dolt-port | 3306 | Dolt SQL port |
| --repo | Shybko/beads-web | GitHub repo for binary |
| --version | latest | beads-web release version or `latest` |

Either `--allow-ip` or `--no-firewall` is required - board has no built-in auth.

**IP management** (standalone, without --database):

| Argument | Default | Description |
|----------|---------|-------------|
| --allow-ip | - | Add IP to allowlist (single IPv4, no CIDR) |
| --remove-ip | - | Remove IP from allowlist |
| --purge-port | - | Drop all ALLOW + REJECT rules for a port (decommission) |
| --port | 3008 | Target port |

### update-remote.sh

| Argument | Default | Description |
|----------|---------|-------------|
| --install-dir | /opt/beads-web | Installation directory |

Reads repo and version from existing `.env` file. No other arguments needed.

## Security

- **No built-in auth** - access controlled via IP allowlist (iptables)
- **No TLS** - use SSH tunnel for encrypted access if needed
- Credentials stored in `.env` (chmod 600, root-only)
- Container runs as non-root user (beadsweb, uid 1000)
- SQL user should have minimal grants (per-database only)

## Troubleshooting

### Container won't start

```bash
cd /opt/beads-web && docker compose logs
```

### Can't connect to Dolt

```bash
# Check dolt-net connectivity
docker exec beads-web curl -sf http://dolt:3306 || echo "No connection"
docker network inspect dolt-net
```

### Port not accessible

```bash
iptables -L DOCKER-USER -n | grep 3008
# Ensure your IP is in the allowlist
```

### Projects disappeared after update

Projects are stored in SQLite at `data/kanban-ui/settings.db`, mounted as a Docker volume. If projects are missing after an update:

```bash
# Check if the volume mount exists
docker inspect beads-web | grep -A 2 kanban-ui

# Check if the DB file exists on host
ls -la /opt/beads-web/data/kanban-ui/settings.db
```

If the DB file exists but isn't mounted, recreate the container:
```bash
cd /opt/beads-web && docker compose down && docker compose up -d
```

If the DB was lost, re-add projects via API (see Quick Start step 3).

### Binary download fails

```bash
# Check latest release URL
curl -fsSL --head "https://github.com/Shybko/beads-web/releases/latest/download/beads-web-linux-x64"
# If 404: check that the repo has at least one release with beads-web-linux-x64 asset
```

### Update doesn't pick up new binary

`update-remote.sh` uses `docker compose build --no-cache` to force re-download. If the binary still seems old:

```bash
# Check which URL is configured
grep RELEASE_URL /opt/beads-web/.env

# Verify the release asset was actually updated
curl -sI "$(grep RELEASE_URL /opt/beads-web/.env | cut -d= -f2-)" | grep last-modified

# Force full rebuild
cd /opt/beads-web && docker compose down && docker compose build --no-cache && docker compose up -d
```