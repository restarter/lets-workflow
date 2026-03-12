# Beads Dolt VPS Setup

How beads connects to the remote Dolt server on VPS.

## Architecture

```
bd (wrapper) → loads .beads/.env → bd-bin (real binary) → VPS MySQL:3306
```

- `~/.local/bin/bd` - shell wrapper script
- `~/.local/bin/bd-bin` - real beads Go binary
- Wrapper walks up from `$PWD`, finds `.beads/.env`, exports vars, calls `bd-bin`
- Works in interactive shell AND Claude Code (non-interactive shells)

## Why This Wrapper

- **beads does NOT read `.beads/.env`** - all `BEADS_DOLT_*` vars read via `os.Getenv()` only (confirmed by source code audit)
- **direnv** doesn't work in Claude Code (each Bash tool = separate non-interactive shell)
- **`.zshrc` functions** also don't work in Claude Code (non-login shell)
- **`metadata.json`** supports host/user but `dolt_server_port` is deprecated, password is env-only
- **Shell wrapper script in PATH** works everywhere

## .beads/.env Format

```
BEADS_DOLT_SERVER_HOST=144.124.255.40
BEADS_DOLT_SERVER_PORT=3306
BEADS_DOLT_SERVER_USER=<user>
BEADS_DOLT_PASSWORD=<password>
```

## .beads/metadata.json (Required)

```json
{
  "database": "dolt",
  "backend": "dolt",
  "dolt_mode": "server",
  "dolt_database": "<database-name>"
}
```

`dolt_mode: server` tells beads to use MySQL protocol instead of embedded mode.
`dolt_database` is per-project (e.g., `lets`, `aff`).

Do NOT put host/port/user here - use `.beads/.env` via the wrapper instead.

## Adding VPS to a New Project

1. `bd init` (creates `.beads/`)
2. Set `dolt_mode: server` and `dolt_database: <name>` in `.beads/metadata.json`
3. Create `.beads/.env` with 4 vars (copy from another project, adjust database if needed)
4. Delete `.beads/dolt-server.port` if it exists (stale file from local mode overrides port)
5. `bd ready` to verify connection

## Updating Beads Binary

After `go install` or downloading a new version:

```bash
cp <new-binary> ~/.local/bin/bd-bin    # NOT bd!
```

**Never overwrite `~/.local/bin/bd`** - that's the wrapper script.

## Projects Without VPS

Projects without `.beads/.env` use local embedded Dolt automatically. No configuration needed.

## Gotchas

- **Stale `.beads/dolt-server.port`** - left over from local mode, overrides the port from `.beads/.env`. Delete it when switching to remote.
- **Port priority in beads**: env var > port file > config.yaml > metadata.json. The port file wins over metadata.json.
- **`bd dolt status` shows "not running"** when connected to VPS - this is correct (no local server).
- **VPS server**: proxy-master at 144.124.255.40 (Amsterdam), Docker Dolt on port 3306.

## The Wrapper Script

Location: `~/.local/bin/bd`

```sh
#!/bin/sh
# bd wrapper - loads .beads/.env for per-project Dolt credentials
# Wraps bd-bin (the real beads binary)

# Find .beads/.env walking up from cwd
dir="$PWD"
while [ "$dir" != "/" ]; do
  if [ -f "$dir/.beads/.env" ]; then
    while IFS='=' read -r key val; do
      case "$key" in
        ''|\#*) continue ;;
      esac
      val=$(echo "$val" | sed "s/^[\"']//;s/[\"']$//")
      export "$key=$val"
    done < "$dir/.beads/.env"
    break
  fi
  dir=$(dirname "$dir")
done

exec bd-bin "$@"
```
