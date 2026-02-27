# Dolt Remote Setup - Research Notes

## Current State (2026-02-27)

### What works
- Script `scripts/setup-dolt-remote.sh` deploys Dolt in Docker on VPS (one command)
- Remote server running at `144.124.255.40:50051` with `beads-demo` repo
- Direct `dolt push` from CLI works: `DOLT_REMOTE_PASSWORD="" dolt push --user root --force origin main`
- `bd dolt remote add origin http://144.124.255.40:50051/beads-demo` - works

### What doesn't work yet
- `bd dolt push` fails because it pushes through local dolt sql-server (not CLI)
- Local server needs `DOLT_REMOTE_USER` and `DOLT_REMOTE_PASSWORD` in its env
- HTTP remotesapi doesn't support password auth - only works with empty password
- When we set `DOLT_REMOTE_PASSWORD=x` and restart local server, remote rejects because root@% has no password

### The fix needed (next session)
1. Set root password on remote: `docker exec dolt dolt sql -q "ALTER USER 'root'@'%' IDENTIFIED BY 'dolt-sync';"`
2. Update script to set `DOLT_ROOT_PASSWORD` in docker-compose env
3. Then locally: `DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD=dolt-sync bd dolt push`
4. Consider adding these to `.beads/config.yaml` or beads-demo env so they persist

## Key Discovery: How bd dolt push works

`bd dolt push` does NOT call `dolt push` CLI. It executes `CALL dolt_push()` through the **local** dolt sql-server. The local server then connects to the remote. So:

```
bd dolt push -> local dolt sql-server -> CALL dolt_push() -> connects to remote:50051
```

The local server needs DOLT_REMOTE_USER/DOLT_REMOTE_PASSWORD in its environment. bd auto-starts the server - it must pass these env vars when starting.

Stopping and restarting with env vars works: `bd dolt stop && DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD=xxx bd dolt push`

## HTTP vs HTTPS remotesapi auth

- **HTTP**: root without password works. Password-based auth does NOT work over HTTP.
- **HTTPS**: Full password auth supported (Basic auth header over TLS)
- The official docs examples use `https://` with passwords
- Blog post example uses `http://` with root and empty password

This means for HTTP: either use root without password + firewall, or set up TLS.

We confirmed: setting a password on remote root and passing it from local client SHOULD work with the `bd dolt stop && restart with env` approach. Not yet tested.

## Architecture

### Remote (VPS: proxy-master 144.124.255.40)
```
/opt/dolt-remote/
├── docker-compose.yml     # dolthub/dolt-sql-server container
├── servercfg.d/
│   └── config.yaml        # remotesapi:50051, data_dir, listener
└── data/
    └── beads-demo/        # dolt init'd repo
```

Container: `dolthub/dolt-sql-server:latest`
- Port 3306 (SQL, localhost)
- Port 50051 (remotesapi, all interfaces)
- `DOLT_ROOT_HOST=%` env var (allows remote root access)
- Entrypoint auto-detects config from `/etc/dolt/servercfg.d/*.yaml`

### Local (beads-demo)
```
reference/beads-demo/.beads/dolt/  # local dolt database
  - remote "origin" -> http://144.124.255.40:50051/beads-demo
  - bd dolt set user root
```

## Docker Image: dolthub/dolt-sql-server

### Environment Variables
| Variable | Default | Description |
|----------|---------|-------------|
| `DOLT_ROOT_PASSWORD` | (empty) | Root user password |
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
- `/var/lib/dolt/` - default data directory
- `.init_completed` - flag file, delete to re-run initdb

### Init gotchas
- `--entrypoint dolt` required for `dolt init` (entrypoint starts sql-server by default)
- `DOLT_SILENCE_USER_REQ_FOR_TESTING=1` + `--name/--email` flags for identity
- `DOLT_USER` created via env vars works, but password auth over HTTP remotesapi fails

## Beads Dolt Integration

### Key env vars for beads (from reference/beads/docs/DOLT.md)
| Variable | Purpose |
|----------|---------|
| `DOLT_REMOTE_USER` | Push/pull auth user |
| `DOLT_REMOTE_PASSWORD` | Push/pull auth password |
| `BEADS_DOLT_PASSWORD` | Server mode password |
| `BEADS_DOLT_SERVER_MODE` | Enable server mode ("1") |
| `BEADS_DOLT_SERVER_HOST` | Server host |
| `BEADS_DOLT_SERVER_PORT` | Server port |
| `BEADS_DOLT_SERVER_USER` | MySQL connection user |

### bd dolt commands
```bash
bd dolt remote add origin <url>   # Add remote
bd dolt remote list               # List remotes
bd dolt push                      # Push to remote (via sql-server)
bd dolt pull                      # Pull from remote
bd dolt commit                    # Commit pending changes
bd dolt show                      # Show config
bd dolt set <key> <value>         # Set config (database, host, port, user, data-dir)
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
- `reference/beads/docs/DOLT.md` - beads federation, credentials, env vars
- `reference/beads/docs/DOLT-BACKEND.md` - sync modes, server config

## TODO for next session

1. Set root password on remote Docker container
2. Test `bd dolt push` with `DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD=<pass>`
3. Find how to persist DOLT_REMOTE_* env vars for bd (config.yaml? .env?)
4. Update `scripts/setup-dolt-remote.sh` with DOLT_ROOT_PASSWORD
5. Test full cycle: create task -> bd dolt push -> bd dolt pull from another clone
6. Set up remote for main lets-workflow project (not just beads-demo)
