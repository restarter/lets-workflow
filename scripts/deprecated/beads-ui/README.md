# beads-ui - Web Dashboard (DEPRECATED)

> **DEPRECATED.** Superseded by [beads-web](../../beads-web/). Use `scripts/beads-web/` for new deploys.
>
> beads-ui is the legacy Node.js dashboard (port 9080). beads-web is the Rust/Axum-based replacement (port 3008): single binary with embedded frontend, faster, broader feature set (Dolt cache, donut chart, labels filter).
>
> This folder is kept for historical reference and to document the VPS cleanup runbook below. Do not use for new installs.

## VPS Cleanup Runbook

If beads-ui is still running on a VPS, decommission it:

```bash
# 1. Stop and remove container + image
ssh root@vps "cd /opt/beads-ui && docker compose down --rmi all -v"

# 2. Drop ALL firewall rules for the port (ALLOWs from any IP + REJECT, v4 + v6).
ssh root@vps "bash -s -- --purge-port 9080" \
  < scripts/deprecated/beads-ui/setup-remote.sh

# 3. Remove install directory
ssh root@vps "rm -rf /opt/beads-ui /opt/beads-ui-*"

# 4. Verify nothing listens on 9080
ssh root@vps "ss -tlnp | grep ':9080'"   # should be empty
ssh root@vps "iptables -L DOCKER-USER -n | grep 9080"  # should be empty
```

The Dolt container and `dolt-net` Docker network are NOT removed - they are shared with beads-web and stay.

---

# Original Documentation (legacy)

Web dashboard for beads issue tracker. Runs in Docker, connects to Dolt SQL server.

## Architecture

```
Developer browser
     |
http://VPS_IP:9080
     |
iptables DOCKER-USER (IP allowlist)
     |
[beads-ui container]  --dolt-net-->  [Dolt container]
   Node.js 22                         MySQL :3306
   beads-ui + bd CLI
```

## Prerequisites

- Dolt server running in Docker (see scripts/dolt/)
- dolt-net Docker network (created automatically by setup script)
- SQL user with grants on target database

## Quick Start

### 1. Create SQL user in Dolt (if not exists)

See [scripts/dolt/README.md - Add SQL user](../../dolt/README.md#add-sql-user).

### 2. Deploy beads-ui

```bash
ssh root@vps "bash -s -- \
  --sql-user bdui --sql-password your-password \
  --database lets --port 9080 \
  --allow-ip YOUR_IP_1 --allow-ip YOUR_IP_2" \
  < scripts/deprecated/beads-ui/setup-remote.sh
```

### 3. Open in browser

```
http://VPS_IP:9080
```

## Adding Another Database

```bash
ssh root@vps "bash -s -- \
  --sql-user bdui --sql-password your-password \
  --database aff --port 9081 \
  --install-dir /opt/beads-ui-aff \
  --allow-ip YOUR_IP" < scripts/deprecated/beads-ui/setup-remote.sh
```

## IP Allowlist

```bash
# Add IP
ssh root@vps "bash -s -- --allow-ip 1.2.3.4 --port 9080" \
  < scripts/deprecated/beads-ui/setup-remote.sh

# Remove IP
ssh root@vps "bash -s -- --remove-ip 1.2.3.4 --port 9080" \
  < scripts/deprecated/beads-ui/setup-remote.sh
```

## Upgrading

```bash
ssh root@vps "bash -s -- \
  --sql-user bdui --sql-password your-password \
  --database lets --port 9080 \
  --version 0.12.0" < scripts/deprecated/beads-ui/setup-remote.sh
```

The script is idempotent - it rebuilds the container with the new version.

## Server Layout

```
/opt/beads-ui/              # Default install dir
├── .env                    # Credentials (chmod 600)
├── docker-compose.yml
├── Dockerfile
└── data/
    └── .beads/
        └── metadata.json   # Database: lets
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
| --port | 9080 | Web UI port |
| --install-dir | /opt/beads-ui | Installation directory |
| --dolt-host | dolt | Dolt container hostname |
| --dolt-port | 3306 | Dolt SQL port |
| --version | 0.11.1 | beads-ui npm version |

Either `--allow-ip` or `--no-firewall` is required - dashboard has no built-in auth.

**IP management** (standalone, without --database):

| Argument | Default | Description |
|----------|---------|-------------|
| --allow-ip | - | Add IP to allowlist |
| --remove-ip | - | Remove IP from allowlist |
| --port | 9080 | Target port |

## Security

- **No built-in auth** - access controlled via IP allowlist (iptables)
- **No TLS** - use SSH tunnel for encrypted access if needed
- Credentials stored in .env (chmod 600, root-only)
- Container runs as non-root user (node)
- SQL user should have minimal grants (per-database only)

## Troubleshooting

### Container won't start
```bash
cd /opt/beads-ui && docker compose logs
```

### Can't connect to Dolt
```bash
docker exec beads-ui-lets bd sql -q "SELECT 1"
# Check dolt-net: docker network inspect dolt-net
```

### Port not accessible
```bash
iptables -L DOCKER-USER -n | grep 9080
# Ensure your IP is in the allowlist
```