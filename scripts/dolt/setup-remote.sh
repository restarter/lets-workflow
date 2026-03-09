#!/bin/bash
# Dolt Remote Server - Docker Installation
# Serves beads databases via Direct SQL (primary) or remotesapi (legacy).
#
# Usage:
#   # Full install with Direct SQL exposed
#   ssh root@vps "bash -s -- --expose-sql lets aff test" < scripts/dolt/setup-remote.sh
#
#   # Full install with custom root password
#   ssh root@vps "bash -s -- --root-password s3cret --expose-sql lets" < scripts/dolt/setup-remote.sh
#
#   # Add database to existing install
#   ssh root@vps "bash -s -- --add-repo new-project" < scripts/dolt/setup-remote.sh
#
#   # Add user to existing install
#   ssh root@vps "bash -s -- --add-user dima:mypass123" < scripts/dolt/setup-remote.sh
#
#   # Manage IP allowlist (for Direct SQL access control)
#   ssh root@vps "bash -s -- --allow-ip 1.2.3.4" < scripts/dolt/setup-remote.sh
#   ssh root@vps "bash -s -- --remove-ip 1.2.3.4" < scripts/dolt/setup-remote.sh
#
# Architecture:
#   One Dolt container serves ALL databases via data_dir volume.
#   Direct SQL: clients connect to port 3306 via MySQL protocol.
#   RemotesAPI: legacy push/pull over HTTP (port 50051, localhost-only when --expose-sql).
#
# Auth:
#   Per-user SQL accounts for both Direct SQL and remotesapi.
#   Each team member gets their own account via --add-user.

set -e

# ============================================
# Configuration
# ============================================
INSTALL_DIR="${INSTALL_DIR:-/opt/dolt-remote}"
REMOTESAPI_PORT="${REMOTESAPI_PORT:-50051}"
SQL_PORT="${SQL_PORT:-3306}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# ============================================
# Helpers
# ============================================
validate_name() {
  local value="$1" label="$2"
  if [[ ! "$value" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    echo -e "${RED}Error: ${label} must be alphanumeric (a-z, 0-9, _, -)${NC}"
    exit 1
  fi
}

validate_password() {
  local value="$1"
  if [[ "$value" =~ [\'\"\;\`\$\\] ]]; then
    echo -e "${RED}Error: Password contains disallowed characters (' \" ; \` \$ \\)${NC}"
    exit 1
  fi
}

# ============================================
# Parse args
# ============================================
ADD_REPO_ONLY=false
ADD_USER=""
ROOT_PASSWORD=""
EXPOSE_SQL=false
ALLOW_IP=""
REMOVE_IP=""
REPOS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --add-repo) ADD_REPO_ONLY=true; shift ;;
    --add-user) ADD_USER="$2"; shift 2 ;;
    --root-password) ROOT_PASSWORD="$2"; shift 2 ;;
    --expose-sql) EXPOSE_SQL=true; shift ;;
    --allow-ip) ALLOW_IP="$2"; shift 2 ;;
    --remove-ip) REMOVE_IP="$2"; shift 2 ;;
    --) shift ;;
    *) REPOS+=("$1"); shift ;;
  esac
done

# ============================================
# IP allowlist management
# ============================================
manage_ip_allowlist() {
  local action="$1" ip="$2" port="${SQL_PORT:-3306}"

  if ! dpkg -s iptables-persistent &>/dev/null; then
    echo "Installing iptables-persistent..."
    DEBIAN_FRONTEND=noninteractive apt-get install -y iptables-persistent
  fi

  if [[ "$action" == "add" ]]; then
    if iptables -C DOCKER-USER -p tcp --dport "$port" -s "$ip" -j ACCEPT 2>/dev/null; then
      echo -e "${YELLOW}IP $ip already allowed${NC}"
      return 0
    fi
    iptables -I DOCKER-USER -p tcp --dport "$port" -s "$ip" -j ACCEPT
    echo -e "${GREEN}Allowed IP: $ip on port $port${NC}"
  elif [[ "$action" == "remove" ]]; then
    iptables -D DOCKER-USER -p tcp --dport "$port" -s "$ip" -j ACCEPT 2>/dev/null || true
    echo -e "${GREEN}Removed IP: $ip${NC}"
  fi

  if ! iptables -C DOCKER-USER -p tcp --dport "$port" -j DROP 2>/dev/null; then
    iptables -A DOCKER-USER -p tcp --dport "$port" -j DROP
  fi

  netfilter-persistent save 2>/dev/null || iptables-save > /etc/iptables/rules.v4

  echo ""
  echo "Current allowlist for port $port:"
  iptables -L DOCKER-USER -n --line-numbers 2>/dev/null | grep -E "dpt:$port|Chain"
}

# --allow-ip mode
if [[ -n "$ALLOW_IP" ]]; then
  manage_ip_allowlist "add" "$ALLOW_IP"
  exit 0
fi

# --remove-ip mode
if [[ -n "$REMOVE_IP" ]]; then
  manage_ip_allowlist "remove" "$REMOVE_IP"
  exit 0
fi

# --add-user mode: create user and exit
if [[ -n "$ADD_USER" ]]; then
  USERNAME="${ADD_USER%%:*}"
  PASSWORD="${ADD_USER#*:}"

  if [[ "$USERNAME" == "$PASSWORD" ]] || [[ -z "$PASSWORD" ]]; then
    echo -e "${RED}Error: Use --add-user name:password${NC}"
    exit 1
  fi

  validate_name "$USERNAME" "Username"
  validate_password "$PASSWORD"

  echo -e "${YELLOW}Creating user '$USERNAME'...${NC}"
  docker exec dolt dolt sql -q "CREATE USER '${USERNAME}'@'%' IDENTIFIED BY '${PASSWORD}';"
  docker exec dolt dolt sql -q "GRANT ALL ON *.* TO '${USERNAME}'@'%';"
  echo -e "${GREEN}  ✓ User '$USERNAME' created with push/pull access${NC}"
  exit 0
fi

# Validate repo names
for repo in "${REPOS[@]}"; do
  validate_name "$repo" "Repo name '$repo'"
done

if [[ ${#REPOS[@]} -eq 0 ]]; then
  echo "Usage: $0 [options] <db-name> [db-name...]"
  echo ""
  echo "Full install (Direct SQL):"
  echo "  $0 --expose-sql lets aff test"
  echo "  $0 --root-password s3cret --expose-sql lets"
  echo ""
  echo "Manage existing:"
  echo "  $0 --add-repo new-project"
  echo "  $0 --add-user dima:mypass123"
  echo "  $0 --allow-ip 1.2.3.4"
  echo "  $0 --remove-ip 1.2.3.4"
  exit 1
fi

# ============================================
# Validate
# ============================================
if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Error: Please run as root (sudo)${NC}"
  exit 1
fi

# Resolve root password: --root-password > existing .env > generate new
if [[ -z "$ROOT_PASSWORD" && -f "$INSTALL_DIR/.env" ]]; then
  ROOT_PASSWORD=$(grep '^ROOT_PASSWORD=' "$INSTALL_DIR/.env" | cut -d= -f2)
  if [[ -n "$ROOT_PASSWORD" ]]; then
    echo -e "${YELLOW}Using existing root password from $INSTALL_DIR/.env${NC}"
  fi
fi
if [[ -z "$ROOT_PASSWORD" ]]; then
  ROOT_PASSWORD=$(openssl rand -base64 18 | tr -d '/+=' | head -c 16)
  echo -e "${YELLOW}Generated root password: ${ROOT_PASSWORD}${NC}"
fi

# Resolve DOLT_VERSION: existing .env > default "latest"
if [[ -z "$DOLT_VERSION" && -f "$INSTALL_DIR/.env" ]]; then
  DOLT_VERSION=$(grep '^DOLT_VERSION=' "$INSTALL_DIR/.env" | cut -d= -f2)
fi
DOLT_VERSION="${DOLT_VERSION:-latest}"

echo ""
echo -e "${GREEN}=== Dolt Remote Server Setup ===${NC}"
echo -e "${GREEN}    Docker + remotesapi${NC}"
echo ""

# ============================================
# 1. Install Docker
# ============================================
install_docker() {
  echo -e "${YELLOW}[1/5] Installing Docker...${NC}"

  if command -v docker &>/dev/null; then
    if docker info &>/dev/null; then
      echo "  Docker already installed and running"
    else
      echo "  Docker installed but not running, starting..."
      systemctl daemon-reload
      systemctl start docker.socket
      systemctl start docker.service
      sleep 2
    fi
  else
    echo "  Installing Docker..."
    curl -fsSL https://get.docker.com | sh
    systemctl start docker
    sleep 2
  fi

  # Docker Compose v2
  if docker compose version &>/dev/null; then
    echo "  Docker Compose v2 ready"
  else
    echo "  Installing Docker Compose plugin..."
    mkdir -p /usr/local/lib/docker/cli-plugins
    curl -SL https://github.com/docker/compose/releases/latest/download/docker-compose-linux-x86_64 \
      -o /usr/local/lib/docker/cli-plugins/docker-compose
    chmod +x /usr/local/lib/docker/cli-plugins/docker-compose
  fi
}

# ============================================
# 2. Create directories and config
# ============================================
create_config() {
  echo -e "${YELLOW}[2/5] Creating configuration...${NC}"
  mkdir -p "$INSTALL_DIR/dolt-home"
  mkdir -p "$INSTALL_DIR/servercfg.d"

  # Dolt server config
  cat > "$INSTALL_DIR/servercfg.d/config.yaml" <<EOF
listener:
  host: "0.0.0.0"
  port: 3306
behavior:
  autocommit: true
remotesapi:
  port: 50051
data_dir: /var/lib/dolt/data
EOF

  # Docker Compose
  # Mount entire /var/lib/dolt so privileges.db, .doltcfg, and .init_completed
  # all persist alongside repo data. This ensures DOLT_ROOT_PASSWORD is applied
  # correctly on first start and preserved across restarts.
  # Password and ports are read from .env file (not inlined).
  if [[ "$EXPOSE_SQL" == true ]]; then
    SQL_BIND='${SQL_PORT}:3306'
    # When SQL is exposed, restrict remotesapi to localhost (not needed externally)
    REMOTESAPI_BIND='127.0.0.1:${REMOTESAPI_PORT}:50051'
  else
    SQL_BIND='127.0.0.1:${SQL_PORT}:3306'
    REMOTESAPI_BIND='${REMOTESAPI_PORT}:50051'
  fi

  cat > "$INSTALL_DIR/docker-compose.yml" <<EOF
services:
  dolt:
    image: dolthub/dolt-sql-server:\${DOLT_VERSION:-latest}
    container_name: dolt
    restart: unless-stopped
    ports:
      - "${SQL_BIND}"
      - "${REMOTESAPI_BIND}"
    volumes:
      - ./dolt-home:/var/lib/dolt
      - ./servercfg.d:/etc/dolt/servercfg.d:ro
    environment:
      - DOLT_ROOT_HOST=%
      - DOLT_ROOT_PASSWORD=\${ROOT_PASSWORD}
    healthcheck:
      test: ["CMD", "dolt", "sql", "-q", "SELECT 1"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 15s
    deploy:
      resources:
        limits:
          memory: 2G
          cpus: "2.0"
        reservations:
          memory: 512M
EOF

  echo "  ✓ Config written to $INSTALL_DIR/"
  if [[ "$EXPOSE_SQL" == true ]]; then
    echo "  ✓ SQL port exposed externally (for direct server mode)"
  else
    echo "  ✓ SQL port bound to localhost only"
  fi
}

# ============================================
# 3. Init repos
# ============================================
init_repos() {
  echo -e "${YELLOW}[3/5] Initializing repos...${NC}"

  for repo in "${REPOS[@]}"; do
    if [ -d "$INSTALL_DIR/dolt-home/data/$repo/.dolt" ]; then
      echo "  - Repo '$repo' already initialized"
    else
      mkdir -p "$INSTALL_DIR/dolt-home/data/$repo"
      docker run --rm \
        --entrypoint dolt \
        -e DOLT_SILENCE_USER_REQ_FOR_TESTING=1 \
        -v "$INSTALL_DIR/dolt-home/data/$repo:/repo" \
        -w /repo \
        dolthub/dolt-sql-server:latest \
        init --name "dolt-remote" --email "dolt@remote"
      echo "  ✓ Repo '$repo' initialized"
    fi
  done
}

# ============================================
# 4. Start containers
# ============================================
start_containers() {
  echo -e "${YELLOW}[4/5] Starting Dolt server...${NC}"
  cd "$INSTALL_DIR"
  docker compose up -d
  sleep 5

  if docker compose ps --format '{{.Status}}' | grep -q "Up"; then
    echo "  ✓ Dolt container running"
  else
    echo -e "${RED}ERROR: Dolt failed to start. Check: docker compose logs${NC}"
    exit 1
  fi
}

# ============================================
# 5. Verify port access
# ============================================
verify_port() {
  echo -e "${YELLOW}[5/5] Checking port access...${NC}"

  # Docker publishes ports via FORWARD/DOCKER chains, bypassing INPUT.
  # iptables INPUT rules have no effect on Docker-published ports.
  # To restrict access, use DOCKER-USER chain:
  #   iptables -I DOCKER-USER -p tcp --dport 50051 -s <allowed-ip> -j ACCEPT
  #   iptables -A DOCKER-USER -p tcp --dport 50051 -j DROP

  echo "  Port $REMOTESAPI_PORT published by Docker (bypasses iptables INPUT)"
  echo "  To restrict by IP, use DOCKER-USER chain (see script comments)"
}

# ============================================
# Summary
# ============================================
print_summary() {
  PUBLIC_IP=$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')

  echo ""
  echo -e "${GREEN}========================================${NC}"
  echo -e "${GREEN}    Dolt Remote Server Ready${NC}"
  echo -e "${GREEN}========================================${NC}"
  echo ""
  echo "  Host:           $PUBLIC_IP"
  if [[ "$EXPOSE_SQL" == true ]]; then
    echo "  SQL:            $PUBLIC_IP:$SQL_PORT (external - use IP allowlist)"
    echo "  RemotesAPI:     localhost:$REMOTESAPI_PORT (local only)"
  else
    echo "  SQL:            localhost:$SQL_PORT (local only)"
    echo "  RemotesAPI:     $PUBLIC_IP:$REMOTESAPI_PORT"
  fi
  echo "  Root password:  (see $INSTALL_DIR/.env)"
  echo "  Data dir:       $INSTALL_DIR/dolt-home/"
  echo "  Databases:      ${REPOS[*]}"
  echo ""
  if [[ "$EXPOSE_SQL" == true ]]; then
    echo "Direct SQL mode (.beads/metadata.json):"
    echo "  {"
    echo "    \"dolt_mode\": \"server\","
    echo "    \"dolt_server_host\": \"$PUBLIC_IP\","
    echo "    \"dolt_server_port\": $SQL_PORT,"
    echo "    \"dolt_server_user\": \"<your-username>\","
    echo "    \"dolt_database\": \"<db-name>\""
    echo "  }"
    echo ""
    echo "Set password: export BEADS_DOLT_PASSWORD=<your-password>"
    echo ""
    echo "Manage IP allowlist:"
    echo "  --allow-ip <IP>    Allow a developer's IP"
    echo "  --remove-ip <IP>   Remove an IP from allowlist"
  else
    echo "Connect from local machine:"
    for repo in "${REPOS[@]}"; do
      echo "  bd dolt remote add origin http://$PUBLIC_IP:$REMOTESAPI_PORT/$repo"
    done
  fi
  echo ""
  echo "Add user:     --add-user dima:pass123"
  echo "Add database: --add-repo new-project"
  echo ""
  echo -e "${GREEN}========================================${NC}"
}

# ============================================
# Save credentials
# ============================================
save_credentials() {
  cat > "$INSTALL_DIR/.env" <<EOF
# Generated by setup-remote.sh
ROOT_PASSWORD=${ROOT_PASSWORD}
REMOTESAPI_PORT=${REMOTESAPI_PORT}
SQL_PORT=${SQL_PORT}
DOLT_VERSION=${DOLT_VERSION:-latest}
EOF
  chmod 600 "$INSTALL_DIR/.env"
  echo "  ✓ Credentials saved to $INSTALL_DIR/.env"
}

# ============================================
# Main
# ============================================
if [[ "$ADD_REPO_ONLY" == true ]]; then
  init_repos
  cd "$INSTALL_DIR" && docker compose restart
  sleep 3
  print_summary
else
  install_docker
  create_config
  save_credentials
  init_repos
  start_containers
  verify_port
  print_summary
fi
