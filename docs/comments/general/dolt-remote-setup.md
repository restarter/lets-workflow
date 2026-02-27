# Dolt Remote Setup - Research & Implementation Notes

## Current State (2026-02-27, session 2)

### What works
- Script `scripts/dolt/setup-remote.sh` deploys Dolt in Docker on VPS (one command)
- Remote server running at `144.124.255.40:50051` with `beads-demo` repo
- **Password auth over HTTP works** - `DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD=xxx bd dolt push`
- `bd dolt push` and `bd dolt pull` both work with credentials
- Script is idempotent - re-run reuses existing password from `.env`
- Per-user accounts via `--add-user name:password`
- Input validation prevents SQL injection and path traversal

### What needs to be done
- Persist `DOLT_REMOTE_USER`/`DOLT_REMOTE_PASSWORD` for bd (so you don't type them every time)
- Test full cycle: create task -> push -> pull from another machine
- Set up remote for main lets-workflow project (not just beads-demo)

## Key Discoveries

### 1. Password auth DOES work over HTTP

Previous session concluded that HTTP remotesapi doesn't support password auth. **This was wrong.**

The actual issue was a Docker volume mount problem - `DOLT_ROOT_PASSWORD` was never applied because `privileges.db` lived in an anonymous Docker volume, not in our mounted directory.

After fixing the volume mount, password auth works fine:
```bash
bd dolt stop && DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD=xxx bd dolt push
```

### 2. How bd dolt push works (double-hop architecture)

`bd dolt push` does NOT call `dolt push` CLI directly. It goes through a local sql-server:

```
bd dolt push -> LOCAL dolt sql-server -> CALL dolt_push() -> remote:50051
```

The **local** server needs `DOLT_REMOTE_USER`/`DOLT_REMOTE_PASSWORD` in its environment. bd auto-starts the server, so these env vars must be set before calling `bd dolt push`.

Pattern that works:
```bash
bd dolt stop && DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD=xxx bd dolt push
```

### 3. The volume mount problem (root cause of auth failures)

The Docker image `dolthub/dolt-sql-server` stores data in different locations:

| What | Path | Purpose |
|------|------|---------|
| Repositories | `/var/lib/dolt/data/` | Actual dolt repos |
| Privileges | `/var/lib/dolt/.doltcfg/privileges.db` | User passwords, grants |
| Init flag | `/var/lib/dolt/.init_completed` | Prevents re-init |
| Config | `/var/lib/dolt/.doltcfg/` | Global dolt config |

**Wrong mount** (session 1):
```yaml
volumes:
  - ./data:/var/lib/dolt/data    # Only repos! Privileges in anonymous volume
```
Result: `privileges.db` lives in an anonymous Docker volume. Password set via `DOLT_ROOT_PASSWORD` gets written there on first init, but on container recreate the anonymous volume may persist with old (empty) password, or get lost entirely. Unpredictable behavior.

**Correct mount** (session 2):
```yaml
volumes:
  - ./dolt-home:/var/lib/dolt    # Everything: repos + privileges + config
```
Result: All state under our control. `DOLT_ROOT_PASSWORD` applied on first init, persisted in `dolt-home/.doltcfg/privileges.db`, survives container restarts and re-creates.

### 4. Dolt auth model (SQL users, not SSH keys)

Dolt remotesapi uses MySQL-style SQL users for authentication. No SSH key auth, no token auth.

- Create users: `CREATE USER 'name'@'%' IDENTIFIED BY 'pass'`
- Grant push/pull: `GRANT ALL ON *.* TO 'name'@'%'` (superuser needed for push)
- Grant pull only: `GRANT CLONE_ADMIN ON *.* TO 'name'@'%'`
- Client side: `DOLT_REMOTE_USER=name DOLT_REMOTE_PASSWORD=pass`

### 5. TLS/HTTPS status

- Self-signed certs fail with Go's x509 validator (`certificate is not standards compliant`)
- Let's Encrypt requires a domain (not available for this VPS)
- Client certificate auth exists since Nov 2025 but complex to set up
- **Conclusion:** HTTP + password is sufficient for internal use

### 6. Docker iptables gotcha

Docker publishes ports via FORWARD/DOCKER chains, completely bypassing iptables INPUT chain. This means:
- `iptables -A INPUT -p tcp --dport 50051 -j ACCEPT` does nothing
- To restrict Docker ports by IP, use DOCKER-USER chain:
```bash
iptables -I DOCKER-USER -p tcp --dport 50051 -s <allowed-ip> -j ACCEPT
iptables -A DOCKER-USER -p tcp --dport 50051 -j DROP
```

## Architecture

### Remote (VPS: proxy-master 144.124.255.40)
```
/opt/dolt-remote/
├── .env                  # ROOT_PASSWORD, ports (chmod 600)
├── docker-compose.yml    # References .env vars (not inlined)
├── servercfg.d/
│   └── config.yaml       # remotesapi:50051, data_dir, listener
└── dolt-home/            # Mounted as /var/lib/dolt (entire dolt home)
    ├── .doltcfg/         # privileges.db, global config
    ├── .init_completed   # Prevents re-init
    └── data/
        └── beads-demo/   # dolt init'd repo
```

Container: `dolthub/dolt-sql-server:latest`
- Port 3306 (SQL) - bound to localhost only
- Port 50051 (remotesapi) - public, password-protected
- `DOLT_ROOT_HOST=%` + `DOLT_ROOT_PASSWORD` from .env
- Config auto-detected from `/etc/dolt/servercfg.d/*.yaml`

### Local (beads-demo)
```
reference/beads-demo/.beads/dolt/  # local dolt database
  - remote "origin" -> http://144.124.255.40:50051/beads-demo
  - bd dolt set user root
```

## Script: setup-dolt-remote.sh

### Usage
```bash
# Full install
ssh root@vps "bash -s -- beads-demo" < scripts/setup-dolt-remote.sh
ssh root@vps "bash -s -- --root-password s3cret beads-demo" < scripts/setup-dolt-remote.sh

# Add repo to existing
ssh root@vps "bash -s -- --add-repo new-project" < scripts/setup-dolt-remote.sh

# Add user
ssh root@vps "bash -s -- --add-user dima:mypass123" < scripts/setup-dolt-remote.sh
```

### What the script does (5 steps)
1. Install Docker + Compose v2
2. Create config (servercfg.d/config.yaml, docker-compose.yml)
3. Save credentials to .env (chmod 600), reuse on re-run
4. Init repos (docker run --entrypoint dolt ... init)
5. Start container, verify running

### Security measures (from code review)
- Input validation: username `^[a-zA-Z0-9_-]+$`, password rejects `' " ; $ \ ``
- No `WITH GRANT OPTION` for added users
- Password not inlined in docker-compose.yml (uses .env interpolation)
- Re-run reads existing password from .env instead of generating new one
- SQL port localhost only

## Docker Image: dolthub/dolt-sql-server

### Environment Variables
| Variable | Default | Description |
|----------|---------|-------------|
| `DOLT_ROOT_PASSWORD` | (empty) | Root user password (applied on first init) |
| `DOLT_ROOT_HOST` | localhost | Root user host (`%` for any) |
| `DOLT_USER` | - | Create additional user |
| `DOLT_PASSWORD` | - | Password for DOLT_USER |
| `DOLT_USER_HOST` | (DOLT_ROOT_HOST) | Host for DOLT_USER |
| `DOLT_DATABASE` | - | Create database on init |
| `DOLT_SERVER_TIMEOUT` | 300 | Startup timeout (seconds) |

### Key paths inside container
- `/etc/dolt/servercfg.d/` - server config yaml (auto-detected)
- `/etc/dolt/doltcfg.d/` - dolt config json
- `/docker-entrypoint-initdb.d/` - SQL files executed on first start
- `/var/lib/dolt/` - dolt home (repos, privileges, config)
- `/var/lib/dolt/.init_completed` - flag file, prevents re-init
- `/var/lib/dolt/.doltcfg/privileges.db` - user passwords and grants

### Init behavior
- `DOLT_ROOT_PASSWORD` applied only when `.init_completed` doesn't exist
- After init, `.init_completed` created to prevent re-running
- To reset: delete `.init_completed` and restart container
- `--entrypoint dolt` required for `dolt init` (default entrypoint starts sql-server)
- `DOLT_SILENCE_USER_REQ_FOR_TESTING=1` needed for non-interactive init

## Beads Dolt Integration

### Key env vars for beads
| Variable | Purpose |
|----------|---------|
| `DOLT_REMOTE_USER` | Push/pull auth user (passed to local sql-server) |
| `DOLT_REMOTE_PASSWORD` | Push/pull auth password |
| `BEADS_DOLT_PASSWORD` | Server mode password |
| `BEADS_DOLT_SERVER_MODE` | Enable server mode ("1") |
| `BEADS_DOLT_SERVER_HOST` | Server host |
| `BEADS_DOLT_SERVER_PORT` | Server port |
| `BEADS_DOLT_SERVER_USER` | MySQL connection user |

### bd dolt commands
```bash
bd dolt remote add origin <url>   # Add remote
bd dolt remote remove <name>      # Remove remote
bd dolt remote list               # List remotes
bd dolt push                      # Push to remote (via sql-server)
bd dolt push --force              # Force push (needed for first push to empty remote)
bd dolt pull                      # Pull from remote
bd dolt commit                    # Commit pending changes
bd dolt show                      # Show config
bd dolt set <key> <value>         # Set config (database, host, port, user, data-dir)
bd dolt stop                      # Stop local sql-server
```

### bd sync is DEPRECATED
`bd sync` is now a no-op. Use `bd dolt push` / `bd dolt pull` directly.

## Documentation Sources

- [Using Remotes - Dolt Docs](https://docs.dolthub.com/sql-reference/version-control/remotes) - remotesapi auth, CLONE_ADMIN, push permissions
- [Dolt SQL Server Push Support (blog)](https://www.dolthub.com/blog/2023-12-29-sql-server-push-support/) - push examples, DOLT_REMOTE_PASSWORD
- [Advanced config.yaml (blog)](https://www.dolthub.com/blog/2024-12-18-advanced-config-yaml/) - remotesapi config, read_only
- [Docker Installation - Dolt Docs](https://docs.dolthub.com/introduction/installation/docker) - Docker setup
- [dolthub/dolt-sql-server - Docker Hub](https://hub.docker.com/r/dolthub/dolt-sql-server) - env vars, entrypoint
- [Root Superuser Changes (blog)](https://www.dolthub.com/blog/2025-01-15-root-superuser-change/) - DOLT_ROOT_HOST, security changes
- [Access Management - Dolt Docs](https://docs.dolthub.com/sql-reference/server/access-management) - CREATE USER, GRANT, privileges
- [Client certificate auth - GitHub #10008](https://github.com/dolthub/dolt/issues/10008) - TLS mutual auth (implemented Nov 2025)
- `reference/beads/docs/DOLT.md` - beads federation, credentials, env vars
- `reference/beads/docs/DOLT-BACKEND.md` - sync modes, server config

## TODO

1. Persist `DOLT_REMOTE_USER`/`DOLT_REMOTE_PASSWORD` for bd (config.yaml? .env? shell alias?)
2. Test full cycle: create task on machine A -> push -> pull on machine B -> verify
3. Set up remote for main lets-workflow project (add repo via `--add-repo lets-workflow`)
4. Consider DOCKER-USER iptables rules if team grows beyond trusted devs
