#!/bin/bash
# beads-web - Kanban Board Docker Installation
# Deploys beads-web container (Rust binary) that connects to existing Dolt server via Docker network.
#
# Usage:
#   # Full install
#   ssh root@vps "bash -s -- \
#     --sql-user bdweb --sql-password secret \
#     --database lets --port 9090 \
#     --allow-ip 1.2.3.4 --allow-ip 5.6.7.8" < scripts/beads-web/setup-remote.sh
#
#   # Different database on different port
#   ssh root@vps "bash -s -- \
#     --sql-user bdweb --sql-password secret \
#     --database aff --port 9091 \
#     --install-dir /opt/beads-web-aff \
#     --allow-ip 1.2.3.4" < scripts/beads-web/setup-remote.sh
#
#   # Custom fork
#   ssh root@vps "bash -s -- \
#     --sql-user bdweb --sql-password secret \
#     --database lets --port 9090 \
#     --repo weselow/beads-web --version 1.0.0 \
#     --allow-ip 1.2.3.4" < scripts/beads-web/setup-remote.sh
#
#   # Manage IP allowlist
#   ssh root@vps "bash -s -- --allow-ip 1.2.3.4 --port 9090" < scripts/beads-web/setup-remote.sh
#   ssh root@vps "bash -s -- --remove-ip 1.2.3.4 --port 9090" < scripts/beads-web/setup-remote.sh
#
# Prerequisites:
#   - Docker installed (use scripts/dolt/setup-remote.sh first)
#   - Dolt container running with dolt-net network
#   - SQL user created in Dolt with grants on target database

set -e

# ============================================
# Configuration
# ============================================
INSTALL_DIR="${INSTALL_DIR:-/opt/beads-web}"
BEADS_WEB_PORT="${BEADS_WEB_PORT:-9090}"
BEADS_WEB_VERSION="${BEADS_WEB_VERSION:-0.9.3}"
BEADS_WEB_REPO="${BEADS_WEB_REPO:-Shybko/beads-web}"
DOLT_HOST="${DOLT_HOST:-dolt}"
DOLT_PORT="${DOLT_PORT:-3306}"

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

validate_ip() {
  local ip="$1"
  if [[ ! "$ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+(/[0-9]+)?$ ]]; then
    echo -e "${RED}Error: Invalid IP address format: $ip${NC}"
    exit 1
  fi
  # Validate octets
  local addr cidr
  IFS='/' read -r addr cidr <<< "$ip"
  IFS='.' read -ra octets <<< "$addr"
  for o in "${octets[@]}"; do
    if (( o > 255 )); then
      echo -e "${RED}Error: Invalid IP octet: $o in $ip${NC}"
      exit 1
    fi
  done
  # Block wide CIDR ranges
  if [[ -n "$cidr" ]] && (( cidr < 24 )); then
    echo -e "${RED}Error: CIDR mask /$cidr is too wide (minimum /24)${NC}"
    exit 1
  fi
  if [[ "$ip" == "0.0.0.0/0" || "$ip" == "0.0.0.0" ]]; then
    echo -e "${RED}Error: Refusing 0.0.0.0 - this would open the port to everyone${NC}"
    exit 1
  fi
}

# ============================================
# Parse args
# ============================================
SQL_USER=""
SQL_PASSWORD=""
DATABASE=""
ALLOW_IPS=()
REMOVE_IP=""
NO_FIREWALL=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sql-user) SQL_USER="$2"; shift 2 ;;
    --sql-password) SQL_PASSWORD="$2"; shift 2 ;;
    --database) DATABASE="$2"; shift 2 ;;
    --port) BEADS_WEB_PORT="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --dolt-host) DOLT_HOST="$2"; shift 2 ;;
    --dolt-port) DOLT_PORT="$2"; shift 2 ;;
    --repo) BEADS_WEB_REPO="$2"; shift 2 ;;
    --version) BEADS_WEB_VERSION="$2"; shift 2 ;;
    --allow-ip) ALLOW_IPS+=("$2"); shift 2 ;;
    --remove-ip) REMOVE_IP="$2"; shift 2 ;;
    --no-firewall) NO_FIREWALL=true; shift ;;
    --) shift ;;
    *) echo -e "${RED}Unknown argument: $1${NC}"; exit 1 ;;
  esac
done

# ============================================
# IP allowlist management
# ============================================
manage_ip_allowlist() {
  local action="$1" ip="$2" port="$BEADS_WEB_PORT"

  if ! dpkg -s iptables-persistent &>/dev/null; then
    echo "Installing iptables-persistent..."
    DEBIAN_FRONTEND=noninteractive apt-get install -y iptables-persistent
  fi

  if [[ "$action" == "add" ]]; then
    if iptables -C DOCKER-USER -p tcp --dport "$port" -s "$ip" -j ACCEPT 2>/dev/null; then
      echo -e "${YELLOW}IP $ip already allowed on port $port${NC}"
      return 0
    fi
    iptables -I DOCKER-USER -p tcp --dport "$port" -s "$ip" -j ACCEPT
    echo -e "${GREEN}Allowed IP: $ip on port $port${NC}"
  elif [[ "$action" == "remove" ]]; then
    iptables -D DOCKER-USER -p tcp --dport "$port" -s "$ip" -j ACCEPT 2>/dev/null || true
    echo -e "${GREEN}Removed IP: $ip from port $port${NC}"
  fi

  # Ensure REJECT rule exists at end (tcp-reset gives immediate "Connection refused")
  if ! iptables -C DOCKER-USER -p tcp --dport "$port" -j REJECT --reject-with tcp-reset 2>/dev/null; then
    # Remove old DROP rule if exists (upgrade path)
    iptables -D DOCKER-USER -p tcp --dport "$port" -j DROP 2>/dev/null || true
    iptables -A DOCKER-USER -p tcp --dport "$port" -j REJECT --reject-with tcp-reset
  fi

  # Block IPv6 for this port
  if ! ip6tables -C DOCKER-USER -p tcp --dport "$port" -j REJECT --reject-with tcp-reset 2>/dev/null; then
    ip6tables -D DOCKER-USER -p tcp --dport "$port" -j DROP 2>/dev/null || true
    ip6tables -A DOCKER-USER -p tcp --dport "$port" -j REJECT --reject-with tcp-reset
  fi

  netfilter-persistent save 2>/dev/null || iptables-save > /etc/iptables/rules.v4

  echo ""
  echo "Current allowlist for port $port:"
  iptables -L DOCKER-USER -n --line-numbers 2>/dev/null | grep -E "dpt:$port|Chain"
}

# --remove-ip mode (standalone)
if [[ -n "$REMOVE_IP" ]]; then
  validate_ip "$REMOVE_IP"
  manage_ip_allowlist "remove" "$REMOVE_IP"
  exit 0
fi

# --allow-ip only mode (no --database = just manage firewall)
if [[ ${#ALLOW_IPS[@]} -gt 0 && -z "$DATABASE" ]]; then
  for ip in "${ALLOW_IPS[@]}"; do
    validate_ip "$ip"
    manage_ip_allowlist "add" "$ip"
  done
  exit 0
fi

# ============================================
# Validate
# ============================================
if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Error: Please run as root${NC}"
  exit 1
fi

if ! command -v docker &>/dev/null || ! docker info &>/dev/null; then
  echo -e "${RED}Error: Docker not installed or not running. Run scripts/dolt/setup-remote.sh first.${NC}"
  exit 1
fi

if [[ -z "$DATABASE" ]]; then
  echo "Modes:"
  echo "  Full install:   $0 --sql-user USER --sql-password PASS --database DB --allow-ip IP"
  echo "  Add IP:         $0 --allow-ip IP --port PORT"
  echo "  Remove IP:      $0 --remove-ip IP --port PORT"
  echo ""
  echo "Full install:"
  echo "  --sql-user USER        SQL username for Dolt connection"
  echo "  --sql-password PASS    SQL password"
  echo "  --database DB          Dolt database name (e.g., lets, aff)"
  echo "  --allow-ip IP          Restrict access to this IP (repeatable)"
  echo "  --no-firewall          Leave port open to all (NOT recommended)"
  echo ""
  echo "  Optional:"
  echo "  --port PORT            Web UI port (default: 9090)"
  echo "  --install-dir DIR      Install directory (default: /opt/beads-web)"
  echo "  --dolt-host HOST       Dolt hostname (default: dolt)"
  echo "  --dolt-port PORT       Dolt SQL port (default: 3306)"
  echo "  --repo OWNER/REPO      GitHub repo for binary (default: Shybko/beads-web)"
  echo "  --version VER          beads-web version (default: 0.9.3)"
  echo ""
  echo "IP management (standalone):"
  echo "  --allow-ip IP          Add IP to allowlist"
  echo "  --remove-ip IP         Remove IP from allowlist"
  echo "  --port PORT            Target port (default: 9090)"
  exit 1
fi

if [[ -z "$SQL_USER" || -z "$SQL_PASSWORD" ]]; then
  echo -e "${RED}Error: --sql-user and --sql-password are required${NC}"
  exit 1
fi

validate_name "$DATABASE" "Database"
validate_name "$SQL_USER" "SQL user"

# Resolve versions from existing .env
if [[ -f "$INSTALL_DIR/.env" ]]; then
  echo -e "${YELLOW}Existing installation found at $INSTALL_DIR${NC}"
  existing_version=$(grep '^BEADS_WEB_VERSION=' "$INSTALL_DIR/.env" | cut -d= -f2)
  existing_repo=$(grep '^BEADS_WEB_REPO=' "$INSTALL_DIR/.env" | cut -d= -f2)
  BEADS_WEB_VERSION="${BEADS_WEB_VERSION:-$existing_version}"
  BEADS_WEB_REPO="${BEADS_WEB_REPO:-$existing_repo}"
fi

# Build release URL
RELEASE_URL="https://github.com/${BEADS_WEB_REPO}/releases/download/v${BEADS_WEB_VERSION}/beads-web-linux-x64"

echo ""
echo -e "${GREEN}=== beads-web Kanban Board Setup ===${NC}"
echo -e "${GREEN}    Docker + Dolt Network${NC}"
echo ""
echo "  Database:    $DATABASE"
echo "  Port:        $BEADS_WEB_PORT"
echo "  Install dir: $INSTALL_DIR"
echo "  Repo:        $BEADS_WEB_REPO"
echo "  Version:     $BEADS_WEB_VERSION"
echo "  Binary URL:  $RELEASE_URL"
echo ""

# ============================================
# 1. Create Docker network
# ============================================
create_network() {
  echo -e "${YELLOW}[1/4] Setting up Docker network...${NC}"

  docker network create dolt-net 2>/dev/null && echo "  Created dolt-net network" || echo "  dolt-net network exists"

  # Connect Dolt container if not already connected
  if docker inspect dolt --format '{{range $net, $_ := .NetworkSettings.Networks}}{{$net}} {{end}}' 2>/dev/null | grep -q dolt-net; then
    echo "  Dolt container already on dolt-net"
  else
    if docker ps --format '{{.Names}}' | grep -q '^dolt$'; then
      docker network connect dolt-net dolt
      echo "  Connected Dolt container to dolt-net"
    else
      echo -e "${YELLOW}  Warning: Dolt container not running. Start Dolt first, then re-run.${NC}"
    fi
  fi
}

# ============================================
# 2. Create configuration
# ============================================
create_config() {
  echo -e "${YELLOW}[2/4] Creating configuration...${NC}"
  mkdir -p "$INSTALL_DIR/data/.beads" "$INSTALL_DIR/data/kanban-ui"
  # Container runs as beadsweb (uid 1000) - data dirs must be writable
  chown -R 1000:1000 "$INSTALL_DIR/data"

  # Dockerfile
  cat > "$INSTALL_DIR/Dockerfile" <<'DOCKERFILE'
FROM debian:trixie-slim

ARG BEADS_WEB_VERSION=0.9.3
ARG BEADS_WEB_REPO=Shybko/beads-web

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fSL -o /usr/local/bin/beads-web \
    "https://github.com/${BEADS_WEB_REPO}/releases/download/v${BEADS_WEB_VERSION}/beads-web-linux-x64" \
    && chmod +x /usr/local/bin/beads-web

RUN useradd --uid 1000 --system --create-home beadsweb \
    && mkdir -p /app/.beads && chown -R beadsweb:beadsweb /app

WORKDIR /app
USER beadsweb

EXPOSE 9090

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=10s \
  CMD curl -sf http://localhost:${PORT:-9090}/ || exit 1

CMD ["beads-web"]
DOCKERFILE

  # docker-compose.yml
  cat > "$INSTALL_DIR/docker-compose.yml" <<EOF
services:
  beads-web:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        BEADS_WEB_VERSION: \${BEADS_WEB_VERSION:-0.9.3}
        BEADS_WEB_REPO: \${BEADS_WEB_REPO:-Shybko/beads-web}
    container_name: beads-web
    restart: unless-stopped
    ports:
      - "\${BEADS_WEB_PORT:-9090}:\${BEADS_WEB_PORT:-9090}"
    volumes:
      - ./data/.beads:/app/.beads
      - ./data/kanban-ui:/home/beadsweb/.local/share/kanban-ui
    environment:
      - PORT=\${BEADS_WEB_PORT:-9090}
      - DOLT_HOST=\${DOLT_HOST:-dolt}
      - DOLT_PORT=\${DOLT_PORT:-3306}
      - DOLT_USER=\${SQL_USER}
      - DOLT_PASSWORD=\${SQL_PASSWORD}
      - NO_BROWSER=1
    networks:
      - dolt-net
    deploy:
      resources:
        limits:
          memory: 256M
          cpus: "1.0"
        reservations:
          memory: 64M

networks:
  dolt-net:
    external: true
EOF

  # metadata.json
  cat > "$INSTALL_DIR/data/.beads/metadata.json" <<EOF
{
  "database": "dolt",
  "backend": "dolt",
  "dolt_mode": "server",
  "dolt_database": "${DATABASE}"
}
EOF

  # .env
  cat > "$INSTALL_DIR/.env" <<EOF
# Generated by setup-remote.sh
SQL_USER='${SQL_USER}'
SQL_PASSWORD='${SQL_PASSWORD}'
DATABASE=${DATABASE}
BEADS_WEB_PORT=${BEADS_WEB_PORT}
BEADS_WEB_VERSION=${BEADS_WEB_VERSION}
BEADS_WEB_REPO=${BEADS_WEB_REPO}
DOLT_HOST=${DOLT_HOST}
DOLT_PORT=${DOLT_PORT}
EOF
  chmod 600 "$INSTALL_DIR/.env"

  echo "  Config written to $INSTALL_DIR/"
}

# ============================================
# 3. Build and start
# ============================================
build_and_start() {
  echo -e "${YELLOW}[3/4] Building and starting beads-web...${NC}"

  # Validate release URL before building
  echo "  Checking release URL..."
  if ! curl -fsSL --head "$RELEASE_URL" >/dev/null 2>&1; then
    echo -e "${RED}ERROR: Release not found at: $RELEASE_URL${NC}"
    echo -e "${RED}Check --repo and --version values.${NC}"
    exit 1
  fi
  echo "  Release URL valid"

  cd "$INSTALL_DIR"
  docker compose build
  docker compose up -d
  sleep 5

  if docker compose ps --format '{{.Status}}' | grep -q "Up"; then
    echo "  beads-web running on port $BEADS_WEB_PORT"
  else
    echo -e "${RED}ERROR: beads-web failed to start. Check: docker compose -f $INSTALL_DIR/docker-compose.yml logs${NC}"
    exit 1
  fi
}

# ============================================
# 4. Setup firewall
# ============================================
setup_firewall() {
  echo -e "${YELLOW}[4/4] Setting up firewall...${NC}"

  if [[ "$NO_FIREWALL" == true ]]; then
    echo -e "${YELLOW}  --no-firewall: skipping IP allowlist. Port $BEADS_WEB_PORT is open to all.${NC}"
    return 0
  fi

  if [[ ${#ALLOW_IPS[@]} -eq 0 ]]; then
    echo -e "${RED}Error: --allow-ip is required (board has no auth). Use --no-firewall to explicitly skip.${NC}"
    exit 1
  fi

  for ip in "${ALLOW_IPS[@]}"; do
    validate_ip "$ip"
    manage_ip_allowlist "add" "$ip"
  done
}

# ============================================
# Summary
# ============================================
print_summary() {
  PUBLIC_IP=$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')

  echo ""
  echo -e "${GREEN}========================================${NC}"
  echo -e "${GREEN}    beads-web Kanban Board Ready${NC}"
  echo -e "${GREEN}========================================${NC}"
  echo ""
  echo "  URL:          http://$PUBLIC_IP:$BEADS_WEB_PORT"
  echo "  Database:     $DATABASE"
  echo "  Container:    beads-web"
  echo "  Install dir:  $INSTALL_DIR"
  echo "  Repo:         $BEADS_WEB_REPO"
  echo "  Version:      $BEADS_WEB_VERSION"
  echo ""
  echo "Manage:"
  echo "  Logs:         docker logs beads-web"
  echo "  Restart:      cd $INSTALL_DIR && docker compose restart"
  echo "  Stop:         cd $INSTALL_DIR && docker compose down"
  echo ""
  echo "IP allowlist:"
  echo "  --allow-ip <IP> --port $BEADS_WEB_PORT"
  echo "  --remove-ip <IP> --port $BEADS_WEB_PORT"
  echo ""
  echo "Add another database:"
  echo "  $0 --sql-user USER --sql-password PASS \\"
  echo "    --database aff --port 9091 \\"
  echo "    --install-dir /opt/beads-web-aff \\"
  echo "    --allow-ip <IP>"
  echo ""
  echo -e "${GREEN}========================================${NC}"
}

# ============================================
# Main
# ============================================
create_network
create_config
build_and_start
setup_firewall
print_summary