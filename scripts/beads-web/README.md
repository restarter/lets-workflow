# beads-web - Kanban Board

Kanban board for beads issue tracker. Rust (Axum) binary with embedded Next.js frontend, runs in Docker, connects to Dolt SQL server.

## Architecture

```
Developer browser
     |
http://VPS_IP:9090
     |
iptables DOCKER-USER (IP allowlist)
     |
[beads-web container]  --dolt-net-->  [Dolt container]
   Debian trixie-slim                 MySQL :3306
   Rust binary (beads-web)
```

## Prerequisites

- Dolt server running in Docker (see scripts/dolt/)
- dolt-net Docker network (created automatically by setup script)
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

### 3. Open in browser

```
http://VPS_IP:9090
```

## Adding Another Database

```bash
ssh root@vps "bash -s -- \
  --sql-user bdweb --sql-password your-password \
  --database aff --port 9091 \
  --install-dir /opt/beads-web-aff \
  --allow-ip YOUR_IP" < scripts/beads-web/setup-remote.sh
```

## IP Allowlist

```bash
# Add IP
ssh root@vps "bash -s -- --allow-ip 1.2.3.4 --port 9090" \
  < scripts/beads-web/setup-remote.sh

# Remove IP
ssh root@vps "bash -s -- --remove-ip 1.2.3.4 --port 9090" \
  < scripts/beads-web/setup-remote.sh
```

## Upgrading

```bash
ssh root@vps "bash -s -- \
  --sql-user bdweb --sql-password your-password \
  --database lets --port 9090 \
  --version 1.0.0" < scripts/beads-web/setup-remote.sh
```

The script is idempotent - it rebuilds the container with the new version.

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
/opt/beads-web/              # Default install dir
├── .env                     # Credentials (chmod 600)
├── docker-compose.yml
├── Dockerfile
└── data/
    └── .beads/
        └── metadata.json    # Database: lets
```

## CLI Reference

**Full install** (all required):

| Argument | Default | Description |
|----------|---------|-------------|
| --sql-user | - | SQL username |
| --sql-password | - | SQL password |
| --database | - | Dolt database name |
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

## Security

- **No built-in auth** - access controlled via IP allowlist (iptables)
- **No TLS** - use SSH tunnel for encrypted access if needed
- Credentials stored in .env (chmod 600, root-only)
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

### Adding a remote Dolt database (project)

The UI "Add Project" dialog currently only supports local filesystem paths. To add a remote Dolt database, use the API directly:

```bash
# Add a project pointing to a Dolt database
docker exec beads-web curl -sf -X POST http://localhost:9090/api/projects \
  -H 'Content-Type: application/json' \
  -d '{"name":"my-project","path":"dolt://mydb"}'

# List all projects
docker exec beads-web curl -sf http://localhost:9090/api/projects
```

The `dolt://` prefix tells beads-web to read/write beads via Dolt SQL instead of the filesystem. Multiple databases on the same Dolt server can each be added as a separate project.

See [Shybko/beads-web#3](https://github.com/Shybko/beads-web/issues/3) for UI support progress.

### Binary download fails
```bash
# Check release URL
curl -fsSL --head "https://github.com/Shybko/beads-web/releases/download/v0.9.3/beads-web-linux-x64"
# If 404: check --repo and --version values
```