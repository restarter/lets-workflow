#!/bin/bash
# Dolt Remote Server - Ad-hoc backup
# Single-tarball snapshot of /opt/dolt-remote/ (data + privileges + .env + compose).
# Stops the dolt container briefly for consistent state.
#
# Use case: operator-initiated snapshot before risky operations (migrations, version
# bumps, ad-hoc DDL). NOT a scheduled/retention tool - VPS-level snapshots cover that.
#
# Usage:
#   # Default: stop container, snapshot to /opt/dolt-remote/backups/
#   ssh root@vps "bash -s" < scripts/remote/dolt/backup-remote.sh
#
#   # Custom output dir
#   ssh root@vps "bash -s -- --output-dir /mnt/backups" < scripts/remote/dolt/backup-remote.sh
#
# Restore: see scripts/remote/dolt/README.md "Restore from backup" section.

set -e
umask 077  # tarball + sidecar files default to 600/700

# ============================================
# Configuration
# ============================================
INSTALL_DIR="${INSTALL_DIR:-/opt/dolt-remote}"
OUTPUT_DIR=""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# ============================================
# Parse args
# ============================================
while [[ $# -gt 0 ]]; do
  case "$1" in
    --install-dir) INSTALL_DIR="$2"; shift 2 ;;
    --output-dir) OUTPUT_DIR="$2"; shift 2 ;;
    --help|-h)
      echo "Usage: $0 [--install-dir DIR] [--output-dir DIR]"
      echo ""
      echo "Ad-hoc backup of Dolt SQL server. Stops container briefly for consistency."
      echo ""
      echo "Options:"
      echo "  --install-dir DIR   Dolt install dir (default: /opt/dolt-remote)"
      echo "  --output-dir DIR    Backup output dir (default: <install-dir>/backups)"
      echo ""
      echo "Output: dolt-backup-YYYYMMDD-HHMMSS.tar.gz (chmod 600, contains .env)"
      exit 0
      ;;
    --) shift ;;
    *) echo -e "${RED}Unknown argument: $1${NC}"; exit 1 ;;
  esac
done

OUTPUT_DIR="${OUTPUT_DIR:-$INSTALL_DIR/backups}"

# ============================================
# Pre-flight
# ============================================
if [ "$EUID" -ne 0 ]; then
  echo -e "${RED}Error: Please run as root${NC}"
  exit 1
fi

if [[ ! -d "$INSTALL_DIR/dolt-home" ]]; then
  echo -e "${RED}Error: $INSTALL_DIR/dolt-home not found. Is dolt installed here?${NC}"
  exit 1
fi

if [[ ! -f "$INSTALL_DIR/docker-compose.yml" ]]; then
  echo -e "${RED}Error: $INSTALL_DIR/docker-compose.yml not found${NC}"
  exit 1
fi

mkdir -p "$OUTPUT_DIR"
chmod 700 "$OUTPUT_DIR"

# ============================================
# Plan the backup
# ============================================
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
TARBALL="$OUTPUT_DIR/dolt-backup-${TIMESTAMP}.tar.gz"

# Compute tar exclude path. If OUTPUT_DIR is inside INSTALL_DIR, build a relative
# path (handles arbitrary nesting). If outside, no exclude needed.
OUTPUT_ABS=$(readlink -f "$OUTPUT_DIR")
INSTALL_ABS=$(readlink -f "$INSTALL_DIR")
if [[ "$OUTPUT_ABS" == "$INSTALL_ABS" ]]; then
  echo -e "${RED}Error: --output-dir must not equal --install-dir (would create a recursive tarball)${NC}"
  exit 1
fi
TAR_EXCLUDES=()
if [[ "$OUTPUT_ABS" == "$INSTALL_ABS"/* ]]; then
  REL="${OUTPUT_ABS#$INSTALL_ABS/}"
  TAR_EXCLUDES+=("--exclude=./${REL}")
fi

echo ""
echo -e "${GREEN}=== Dolt Backup ===${NC}"
echo ""
echo "  Install dir: $INSTALL_DIR"
echo "  Output:      $TARBALL"
echo ""

# ============================================
# Stop, snapshot, restart - with safety trap
# ============================================
cd "$INSTALL_DIR"

echo -e "${YELLOW}[1/3] Stopping dolt container...${NC}"
docker compose stop dolt
STOP_TS=$(date +%s)

# Trap: if anything below fails, ensure dolt is restarted before script exits.
# Cleared after successful start.
restart_on_exit() {
  local rc=$?
  if [[ $rc -ne 0 ]]; then
    echo -e "${RED}Backup failed (exit $rc). Restarting dolt container...${NC}"
    docker compose start dolt 2>/dev/null || \
      echo -e "${RED}CRITICAL: dolt restart also failed - manual intervention needed${NC}"
    rm -f "$TARBALL" 2>/dev/null  # don't leave a partial tarball
  fi
}
trap restart_on_exit EXIT

echo -e "${YELLOW}[2/3] Creating tarball...${NC}"
tar -czf "$TARBALL" "${TAR_EXCLUDES[@]}" -C "$INSTALL_DIR" .
chmod 600 "$TARBALL"

echo -e "${YELLOW}[3/3] Restarting dolt container...${NC}"
docker compose start dolt
START_TS=$(date +%s)
DOWNTIME=$((START_TS - STOP_TS))

trap - EXIT  # success path - cancel safety trap

# Wait for healthy - 60s budget covers start_period (15s) + first interval (30s).
echo "  Waiting for dolt to become healthy..."
STATUS=""
for _ in $(seq 1 30); do
  sleep 2
  STATUS=$(docker inspect dolt --format '{{.State.Health.Status}}' 2>/dev/null || echo "")
  if [[ "$STATUS" == "healthy" ]]; then
    break
  fi
done
if [[ "$STATUS" != "healthy" ]]; then
  echo -e "${RED}ERROR: dolt did not reach healthy state in 60s. Backup tarball is OK,${NC}"
  echo -e "${RED}  but check 'docker logs dolt' before relying on the running container.${NC}"
  exit 1
fi

# Sanity-verify the tarball is readable. Cheap (~50ms per 50MB).
if ! gzip -t "$TARBALL" 2>/dev/null; then
  echo -e "${RED}ERROR: tarball failed gzip integrity check.${NC}"
  echo -e "${RED}  This is likely truncation (disk full?). Tarball at: $TARBALL${NC}"
  exit 1
fi

SIZE=$(du -h "$TARBALL" | awk '{print $1}')
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}    Dolt Backup Complete${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "  File:        $TARBALL"
echo "  Size:        $SIZE"
echo "  Downtime:    ${DOWNTIME}s"
echo ""
echo "Restore: see scripts/remote/dolt/README.md 'Restore from backup' section."
echo "Manage:  delete unwanted backups with 'rm $OUTPUT_DIR/dolt-backup-*.tar.gz'"
echo -e "${GREEN}========================================${NC}"
