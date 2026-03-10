#!/bin/bash
# beads-ui - Web Dashboard Docker Installation
# Deploys beads-ui container that connects to existing Dolt server via Docker network.
#
# Usage:
#   # Full install
#   ssh root@vps "bash -s -- \
#     --sql-user bdui --sql-password secret \
#     --database lets --port 9080 \
#     --allow-ip 1.2.3.4 --allow-ip 5.6.7.8" < scripts/beads-ui/setup-remote.sh
#
#   # Different database on different port
#   ssh root@vps "bash -s -- \
#     --sql-user bdui --sql-password secret \
#     --database aff --port 9081 \
#     --install-dir /opt/beads-ui-aff \
#     --allow-ip 1.2.3.4" < scripts/beads-ui/setup-remote.sh
#
#   # Manage IP allowlist
#   ssh root@vps "bash -s -- --allow-ip 1.2.3.4 --port 9080" < scripts/beads-ui/setup-remote.sh
#   ssh root@vps "bash -s -- --remove-ip 1.2.3.4 --port 9080" < scripts/beads-ui/setup-remote.sh
#
# Prerequisites:
#   - Docker installed (use scripts/dolt/setup-remote.sh first)
#   - Dolt container running with dolt-net network
#   - SQL user created in Dolt with grants on target database

set -e

# ============================================
# Configuration
# ============================================
INSTALL_DIR="${INSTALL_DIR:-/opt/beads-ui}"
BEADS_UI_PORT="${BEADS_UI_PORT:-9080}"
BEADS_UI_VERSION="${BEADS_UI_VERSION:-0.11.1}"
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

while [[ $# -gt 0 ]]; do
  case "$1" in
    --sql-user) SQL_USER="$2"; shift 2 ;;
    --sql-password) SQL_PASSWORD="$2"; shift 2 ;;
    --database) DATABASE="$2"; shift 2 ;;
    --port) BEADS_UI_PORT="$2"; shift 2 ;;
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --dolt-host) DOLT_HOST="$2"; shift 2 ;;
    --dolt-port) DOLT_PORT="$2"; shift 2 ;;
    --version) BEADS_UI_VERSION="$2"; shift 2 ;;
    --allow-ip) ALLOW_IPS+=("$2"); shift 2 ;;
    --remove-ip) REMOVE_IP="$2"; shift 2 ;;
    --) shift ;;
    *) echo -e "${RED}Unknown argument: $1${NC}"; exit 1 ;;
  esac
done

# ============================================
# IP allowlist management
# ============================================
manage_ip_allowlist() {
  local action="$1" ip="$2" port="$BEADS_UI_PORT"

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

  # Ensure DROP rule exists at end
  if ! iptables -C DOCKER-USER -p tcp --dport "$port" -j DROP 2>/dev/null; then
    iptables -A DOCKER-USER -p tcp --dport "$port" -j DROP
  fi

  # Block IPv6 for this port
  if ! ip6tables -C DOCKER-USER -p tcp --dport "$port" -j DROP 2>/dev/null; then
    ip6tables -A DOCKER-USER -p tcp --dport "$port" -j DROP
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
  echo "Usage: $0 --sql-user USER --sql-password PASS --database DB [options]"
  echo ""
  echo "Required:"
  echo "  --sql-user USER        SQL username for Dolt connection"
  echo "  --sql-password PASS    SQL password"
  echo "  --database DB          Dolt database name (e.g., lets, aff)"
  echo ""
  echo "Optional:"
  echo "  --port PORT            Web UI port (default: 9080)"
  echo "  --install-dir DIR      Install directory (default: /opt/beads-ui)"
  echo "  --dolt-host HOST       Dolt hostname (default: dolt)"
  echo "  --dolt-port PORT       Dolt SQL port (default: 3306)"
  echo "  --version VER          beads-ui version (default: 0.11.1)"
  echo "  --allow-ip IP          Add IP to allowlist (repeatable)"
  echo ""
  echo "IP management:"
  echo "  --allow-ip IP --port PORT    Add IP for specific port"
  echo "  --remove-ip IP --port PORT   Remove IP from allowlist"
  exit 1
fi

if [[ -z "$SQL_USER" || -z "$SQL_PASSWORD" ]]; then
  echo -e "${RED}Error: --sql-user and --sql-password are required${NC}"
  exit 1
fi

# Resolve versions from existing .env
if [[ -f "$INSTALL_DIR/.env" ]]; then
  echo -e "${YELLOW}Existing installation found at $INSTALL_DIR${NC}"
  existing_version=$(grep '^BEADS_UI_VERSION=' "$INSTALL_DIR/.env" | cut -d= -f2)
  BEADS_UI_VERSION="${BEADS_UI_VERSION:-$existing_version}"
fi

echo ""
echo -e "${GREEN}=== beads-ui Web Dashboard Setup ===${NC}"
echo -e "${GREEN}    Docker + Dolt Network${NC}"
echo ""
echo "  Database:    $DATABASE"
echo "  Port:        $BEADS_UI_PORT"
echo "  Install dir: $INSTALL_DIR"
echo "  Version:     $BEADS_UI_VERSION"
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
  mkdir -p "$INSTALL_DIR/data/.beads"

  # Dockerfile
  cat > "$INSTALL_DIR/Dockerfile" <<'DOCKERFILE'
FROM node:22-slim

ARG BEADS_UI_VERSION=0.11.1

RUN apt-get update \
    && apt-get install -y --no-install-recommends curl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL https://beads.sh | bash \
    && mv /root/.local/bin/bd /usr/local/bin/bd \
    && chmod +x /usr/local/bin/bd

RUN npm install -g beads-ui@${BEADS_UI_VERSION}

RUN mkdir -p /app/.beads && chown -R node:node /app

WORKDIR /app
USER node

ENV BD_BIN=/usr/local/bin/bd

HEALTHCHECK --interval=30s --timeout=5s --retries=3 --start-period=15s \
  CMD node -e "fetch('http://localhost:'+(process.env.PORT||9080)+'/').then(r=>{if(!r.ok)process.exit(1)}).catch(()=>process.exit(1))"

CMD ["sh", "-c", "exec bdui start --host 0.0.0.0 --port ${PORT:-9080}"]
DOCKERFILE

  # docker-compose.yml
  cat > "$INSTALL_DIR/docker-compose.yml" <<EOF
services:
  beads-ui:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        BEADS_UI_VERSION: \${BEADS_UI_VERSION:-0.11.1}
    container_name: beads-ui-${DATABASE}
    restart: unless-stopped
    ports:
      - "\${BEADS_UI_PORT:-9080}:\${BEADS_UI_PORT:-9080}"
    volumes:
      - ./data/.beads:/app/.beads
    environment:
      - PORT=\${BEADS_UI_PORT:-9080}
      - BD_BIN=/usr/local/bin/bd
      - BEADS_DOLT_SERVER_HOST=\${DOLT_HOST:-dolt}
      - BEADS_DOLT_SERVER_PORT=\${DOLT_PORT:-3306}
      - BEADS_DOLT_SERVER_USER=\${SQL_USER}
      - BEADS_DOLT_PASSWORD=\${SQL_PASSWORD}
    networks:
      - dolt-net
    deploy:
      resources:
        limits:
          memory: 512M
          cpus: "1.0"
        reservations:
          memory: 128M

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
SQL_USER=${SQL_USER}
SQL_PASSWORD=${SQL_PASSWORD}
DATABASE=${DATABASE}
BEADS_UI_PORT=${BEADS_UI_PORT}
BEADS_UI_VERSION=${BEADS_UI_VERSION}
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
  echo -e "${YELLOW}[3/4] Building and starting beads-ui...${NC}"
  cd "$INSTALL_DIR"
  docker compose build
  docker compose up -d
  sleep 5

  if docker compose ps --format '{{.Status}}' | grep -q "Up"; then
    echo "  beads-ui-${DATABASE} running on port $BEADS_UI_PORT"
  else
    echo -e "${RED}ERROR: beads-ui failed to start. Check: docker compose -f $INSTALL_DIR/docker-compose.yml logs${NC}"
    exit 1
  fi
}

# ============================================
# 4. Setup firewall
# ============================================
setup_firewall() {
  echo -e "${YELLOW}[4/4] Setting up firewall...${NC}"

  if [[ ${#ALLOW_IPS[@]} -gt 0 ]]; then
    for ip in "${ALLOW_IPS[@]}"; do
      validate_ip "$ip"
      manage_ip_allowlist "add" "$ip"
    done
  else
    echo "  No --allow-ip specified. Port $BEADS_UI_PORT is open."
    echo "  Add IP restrictions: $0 --allow-ip <IP> --port $BEADS_UI_PORT"
  fi
}

# ============================================
# Summary
# ============================================
print_summary() {
  PUBLIC_IP=$(curl -s ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')

  echo ""
  echo -e "${GREEN}========================================${NC}"
  echo -e "${GREEN}    beads-ui Dashboard Ready${NC}"
  echo -e "${GREEN}========================================${NC}"
  echo ""
  echo "  URL:          http://$PUBLIC_IP:$BEADS_UI_PORT"
  echo "  Database:     $DATABASE"
  echo "  Container:    beads-ui-$DATABASE"
  echo "  Install dir:  $INSTALL_DIR"
  echo "  Version:      $BEADS_UI_VERSION"
  echo ""
  echo "Manage:"
  echo "  Logs:         docker logs beads-ui-$DATABASE"
  echo "  Restart:      cd $INSTALL_DIR && docker compose restart"
  echo "  Stop:         cd $INSTALL_DIR && docker compose down"
  echo ""
  echo "IP allowlist:"
  echo "  --allow-ip <IP> --port $BEADS_UI_PORT"
  echo "  --remove-ip <IP> --port $BEADS_UI_PORT"
  echo ""
  echo "Add another database:"
  echo "  $0 --sql-user USER --sql-password PASS \\"
  echo "    --database aff --port 9081 \\"
  echo "    --install-dir /opt/beads-ui-aff \\"
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