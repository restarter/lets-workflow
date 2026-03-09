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

One dolt container serves all project databases. Each project has its own database, accessed by the same SQL user. IP allowlist restricts access to known developer IPs.

## VPS Setup

### Initial deploy

```bash
ssh root@vps "bash -s -- --expose-sql lets aff test" < scripts/dolt/setup-remote.sh
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

Connect as root and create a user:

```bash
# Get root password
ssh root@vps "grep ROOT_PASSWORD /opt/dolt-remote/.env"

# Create user (from any machine that can connect)
mysql -h <vps-ip> -u root -p<root-password> -e "
  CREATE USER 'username'@'%' IDENTIFIED BY 'password';
  GRANT ALL ON *.* TO 'username'@'%';
  FLUSH PRIVILEGES;
"
```

**Important:** Create users through the MySQL protocol connection (not via `docker exec dolt dolt sql`). Users created through the CLI are not visible to the SQL server.

### Add database

```bash
ssh root@vps "bash -s -- --add-repo new-project" < scripts/dolt/setup-remote.sh
```

## IP Allowlist

Access to SQL port 3306 is restricted by IP. Only allowed IPs can connect.

```bash
# Allow an IP
ssh root@vps "bash -s -- --allow-ip 1.2.3.4" < scripts/dolt/setup-remote.sh

# Remove an IP
ssh root@vps "bash -s -- --remove-ip 1.2.3.4" < scripts/dolt/setup-remote.sh

# Check current rules
ssh root@vps "iptables -L DOCKER-USER -n --line-numbers | grep 3306"
```

Rules persist across reboots via `iptables-persistent`.

## Client Setup

### 1. Configure beads

Edit `.beads/metadata.json` in your project root:

```json
{
  "database": "dolt",
  "backend": "dolt",
  "dolt_mode": "server",
  "dolt_server_host": "<vps-ip>",
  "dolt_server_port": 3306,
  "dolt_server_user": "<username>",
  "dolt_database": "<project-database>"
}
```

### 2. Set password

#### Linux / macOS (bash)

```bash
echo 'export BEADS_DOLT_PASSWORD=<password>' >> ~/.bashrc
source ~/.bashrc
```

#### Linux / macOS (zsh)

```bash
echo 'export BEADS_DOLT_PASSWORD=<password>' >> ~/.zshrc
source ~/.zshrc
```

#### Windows (PowerShell, run as Administrator)

```powershell
setx BEADS_DOLT_PASSWORD "<password>"
```

Restart the terminal after running `setx`.

#### Windows (CMD)

```cmd
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
2. **Get credentials:** Admin creates SQL user or shares existing credentials
3. **Configure:** Create `.beads/metadata.json` with server details (see Client Setup)
4. **Set password:** Add `BEADS_DOLT_PASSWORD` to shell profile (see Client Setup)
5. **Verify:** Run `bd list` - should show project tasks

## Adding New Project

1. Add database on VPS:
   ```bash
   ssh root@vps "bash -s -- --add-repo my-project" < scripts/dolt/setup-remote.sh
   ```
2. In the new project, run `bd init` to create `.beads/` directory
3. Configure `.beads/metadata.json` with `"dolt_database": "my-project"`
4. Set `BEADS_DOLT_PASSWORD` if not already in shell profile
5. Run `bd list` to verify

## Backup

Daily backup cron on VPS (keeps 7 days):

```bash
ssh root@vps "mkdir -p /opt/dolt-backups && \
  (crontab -l 2>/dev/null; echo '0 3 * * * tar czf /opt/dolt-backups/\$(date +\%F).tar.gz -C /opt/dolt-remote dolt-home/ && find /opt/dolt-backups -mtime +7 -delete >> /var/log/dolt-backup.log 2>&1') | crontab -"
```

Verify:
```bash
ssh root@vps "crontab -l | grep dolt"
```

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
- **No TLS** - acceptable for trusted developer network with IP allowlist. For TLS: needs a domain (Let's Encrypt) or reverse proxy

## Troubleshooting

### "No authentication methods available for authentication"

User was created via `docker exec dolt dolt sql` instead of through SQL protocol. Recreate the user by connecting as root via MySQL:

```bash
mysql -h <vps-ip> -u root -p<root-password> -e "
  CREATE USER IF NOT EXISTS 'username'@'%' IDENTIFIED BY 'password';
  GRANT ALL ON *.* TO 'username'@'%';
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

The older push/pull architecture is still available via remotesapi on port 50051 (localhost only when `--expose-sql` is used). See `scripts/beads/setup-beads-remote.sh` for the client-side setup. This mode requires a local dolt server and is no longer recommended.