# beads-web - Kanban Board

Kanban board for beads issue tracker. Rust (Axum) binary with embedded Next.js frontend, runs in Docker, connects to Dolt SQL server.

Source: [Shybko/beads-web](https://github.com/Shybko/beads-web) (fork of weselow/beads-web)

## Architecture

```
Developer browser
     |
http://VPS_IP:9090
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
  --database lets --port 9090 \
  --allow-ip YOUR_IP_1 --allow-ip YOUR_IP_2" \
  < scripts/beads-web/setup-remote.sh
```

### 3. Add projects

beads-web supports multiple Dolt databases as separate projects. After deploy, add them via API:

```bash
# Add a project (from VPS)
docker exec beads-web curl -sf -X POST http://localhost:9090/api/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-project","path":"dolt://mydb"}'

# Add another project
docker exec beads-web curl -sf -X POST http://localhost:9090/api/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"another-project","path":"dolt://anotherdb"}'
```

The `dolt://` prefix tells beads-web to read/write via Dolt SQL. Each database on the Dolt server can be a separate project.

> **Note:** The UI "Add Project" button currently only supports local paths. Remote Dolt databases must be added via API. See [Shybko/beads-web#3](https://github.com/Shybko/beads-web/issues/3) for UI support progress.

### 4. Open in browser

```
http://VPS_IP:9090
```

## Updating

When the fork has new changes (binary re-uploaded to the same release tag):

```bash
ssh root@vps "bash -s" < scripts/beads-web/update-remote.sh
```

This rebuilds the Docker image with `--no-cache` (re-downloads the binary) and restarts the container. Project configuration (SQLite DB) is preserved via volume mount.

### Update workflow

1. Push changes to the fork (`Shybko/beads-web`)
2. Build the binary (locally or in a Claude session in the fork repo)
3. Re-upload to the same release tag:
   ```bash
   gh release upload v0.9.3 beads-web-linux-x64 --repo Shybko/beads-web --clobber
   ```
4. Update VPS:
   ```bash
   ssh root@vps "bash -s" < scripts/beads-web/update-remote.sh
   ```

### Version upgrade

To upgrade to a new version (new release tag):

```bash
ssh root@vps "bash -s -- \
  --sql-user bdweb --sql-password your-password \
  --database lets --port 9090 \
  --version 1.0.0" < scripts/beads-web/setup-remote.sh
```

The setup script is idempotent - it regenerates config and rebuilds the container.

## IP Allowlist

```bash
# Add IP
ssh root@vps "bash -s -- --allow-ip 1.2.3.4 --port 9090" \
  < scripts/beads-web/setup-remote.sh

# Remove IP
ssh root@vps "bash -s -- --remove-ip 1.2.3.4 --port 9090" \
  < scripts/beads-web/setup-remote.sh
```

Multiple IPs can be added in a single command:

```bash
ssh root@vps "bash -s -- \
  --allow-ip 1.2.3.4 --allow-ip 5.6.7.8 --allow-ip 9.10.11.12 \
  --port 9090" < scripts/beads-web/setup-remote.sh
```

## Fork Support

By default, the binary is downloaded from `Shybko/beads-web`. Use `--repo` to specify a different fork:

```bash
ssh root@vps "bash -s -- \
  --sql-user bdweb --sql-password your-password \
  --database lets --port 9090 \
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
| --allow-ip | - | Restrict access to this IP (repeatable) |
| --no-firewall | - | Leave port open to all (NOT recommended) |
| --port | 9090 | Web UI port |
| --install-dir | /opt/beads-web | Installation directory |
| --dolt-host | dolt | Dolt container hostname |
| --dolt-port | 3306 | Dolt SQL port |
| --repo | Shybko/beads-web | GitHub repo for binary |
| --version | 0.9.3 | beads-web release version |

Either `--allow-ip` or `--no-firewall` is required - board has no built-in auth.

**IP management** (standalone, without --database):

| Argument | Default | Description |
|----------|---------|-------------|
| --allow-ip | - | Add IP to allowlist |
| --remove-ip | - | Remove IP from allowlist |
| --port | 9090 | Target port |

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
iptables -L DOCKER-USER -n | grep 9090
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
# Check release URL
curl -fsSL --head "https://github.com/Shybko/beads-web/releases/download/v0.9.3/beads-web-linux-x64"
# If 404: check --repo and --version values
```

### Update doesn't pick up new binary

`update-remote.sh` uses `docker compose build --no-cache` to force re-download. If the binary still seems old:

```bash
# Verify the release asset was actually updated
curl -sI "https://github.com/Shybko/beads-web/releases/download/v0.9.3/beads-web-linux-x64" | grep last-modified

# Force full rebuild
cd /opt/beads-web && docker compose down && docker compose build --no-cache && docker compose up -d
```