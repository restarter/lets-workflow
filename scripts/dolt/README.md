# Dolt Remote Server

Deploy and manage a Dolt remotesapi server for syncing beads issue tracker between developers.

## Quick Start

```bash
# Deploy on VPS (one command)
ssh root@your-vps "bash -s -- beads-demo" < scripts/dolt/setup-remote.sh
```

This installs Docker, creates a Dolt container with remotesapi on port 50051, generates a root password, and initializes the `beads-demo` repo.

## Setup

### 1. Deploy

```bash
# With auto-generated password
ssh root@vps "bash -s -- beads-demo" < scripts/dolt/setup-remote.sh

# With custom password
ssh root@vps "bash -s -- --root-password s3cret beads-demo" < scripts/dolt/setup-remote.sh

# Multiple repos at once
ssh root@vps "bash -s -- beads-demo lets-workflow" < scripts/dolt/setup-remote.sh
```

After deploy, root password is saved to `/opt/dolt-remote/.env` on the server.

### 2. Connect locally

```bash
bd dolt remote add origin http://<vps-ip>:50051/beads-demo
```

### 3. Push / Pull

```bash
# Stop local server, set credentials, push
bd dolt stop && DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD='<pass>' bd dolt push

# First push to empty remote needs --force
bd dolt stop && DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD='<pass>' bd dolt push --force

# Pull
bd dolt stop && DOLT_REMOTE_USER=root DOLT_REMOTE_PASSWORD='<pass>' bd dolt pull
```

## Managing Users

Each developer gets their own SQL account:

```bash
# Add user
ssh root@vps "bash -s -- --add-user dima:mypass123" < scripts/dolt/setup-remote.sh

# That user can now push/pull:
DOLT_REMOTE_USER=dima DOLT_REMOTE_PASSWORD=mypass123 bd dolt push
```

Username: alphanumeric, underscore, dash only.
Password: no `' " ; $ \ `` characters.

## Managing Repos

```bash
# Add repo to existing server
ssh root@vps "bash -s -- --add-repo new-project" < scripts/dolt/setup-remote.sh
```

## Re-running the Script

The script is idempotent:
- Reuses existing password from `/opt/dolt-remote/.env`
- Skips already-initialized repos
- Skips Docker install if already present
- Does not recreate running containers

## Architecture

```
Developer A                          VPS (:50051)                    Developer B
     |                                   |                               |
 bd dolt push                    dolthub/dolt-sql-server            bd dolt pull
     |                                   |                               |
 local sql-server ----HTTP----> remotesapi (password auth) <---- local sql-server
     |                                   |                               |
 CALL dolt_push()               /opt/dolt-remote/dolt-home/      CALL dolt_pull()
                                    ├── .doltcfg/privileges.db
                                    └── data/beads-demo/
```

Auth flow: `bd dolt push` starts a local dolt sql-server, which connects to the remote via HTTP with `DOLT_REMOTE_USER`/`DOLT_REMOTE_PASSWORD`.

## Server Layout

```
/opt/dolt-remote/
├── .env                  # Credentials (chmod 600)
├── docker-compose.yml    # Container config (reads .env)
├── servercfg.d/
│   └── config.yaml       # Dolt server config
└── dolt-home/            # Mounted as /var/lib/dolt
    ├── .doltcfg/         # Privileges, global config
    ├── .init_completed   # Init flag
    └── data/
        └── beads-demo/   # Repo data
```

## Ports

| Port | Binding | Purpose |
|------|---------|---------|
| 3306 | localhost only | SQL (MySQL protocol) |
| 50051 | all interfaces | remotesapi (push/pull) |

## Security Notes

- Passwords transmitted over HTTP (plaintext). Acceptable for internal use with trusted network.
- SQL port bound to localhost - not accessible from outside.
- To restrict remotesapi by IP, use Docker's DOCKER-USER iptables chain (not INPUT - Docker bypasses it):
  ```bash
  iptables -I DOCKER-USER -p tcp --dport 50051 -s <allowed-ip> -j ACCEPT
  iptables -A DOCKER-USER -p tcp --dport 50051 -j DROP
  ```
- For TLS: needs a domain (Let's Encrypt) or reverse proxy. Self-signed certs fail with Go's x509 validator.

## Troubleshooting

### "Access denied for user 'root'"
Password mismatch. Check `/opt/dolt-remote/.env` for the actual password.

### "no common ancestor"
First push to an empty remote. Use `--force`:
```bash
bd dolt push --force
```

### "remote 'origin' not found"
Add the remote first:
```bash
bd dolt remote add origin http://<vps-ip>:50051/<repo-name>
```

### Container not starting
```bash
ssh root@vps "cd /opt/dolt-remote && docker compose logs"
```

### Reset everything
```bash
ssh root@vps "cd /opt/dolt-remote && docker compose down -v && rm -rf dolt-home && rm .env"
# Then re-run the script
```
