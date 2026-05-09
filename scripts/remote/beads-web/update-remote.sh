#!/bin/bash
# beads-web - Update binary on VPS
# Downloads the latest binary from GitHub Releases and restarts the container.
# The release asset can be updated without bumping version via:
#   gh release upload <tag> beads-web-linux-x64 --repo Shybko/beads-web --clobber
# Or if the fork has GitHub Actions, just create a new release from GitHub UI.
#
# Usage:
#   ssh root@vps "bash -s" < scripts/remote/beads-web/update-remote.sh
#   ssh root@vps "bash -s -- --install-dir /opt/beads-web-custom" < scripts/remote/beads-web/update-remote.sh
#
# Prerequisites:
#   - beads-web already installed via setup-remote.sh

set -e

# ============================================
# Configuration
# ============================================
INSTALL_DIR="${INSTALL_DIR:-/opt/beads-web}"

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
    --) shift ;;
    *) echo -e "${RED}Unknown argument: $1${NC}"; exit 1 ;;
  esac
done

# ============================================
# Validate
# ============================================
if [[ ! -f "$INSTALL_DIR/.env" ]]; then
  echo -e "${RED}Error: No installation found at $INSTALL_DIR${NC}"
  echo "Run setup-remote.sh first."
  exit 1
fi

# Read config from existing .env
BEADS_WEB_VERSION=$(grep '^BEADS_WEB_VERSION=' "$INSTALL_DIR/.env" | cut -d= -f2)
BEADS_WEB_REPO=$(grep '^BEADS_WEB_REPO=' "$INSTALL_DIR/.env" | cut -d= -f2)
RELEASE_URL=$(grep '^RELEASE_URL=' "$INSTALL_DIR/.env" | cut -d= -f2-)

# Fallback: build URL from version/repo if RELEASE_URL not in .env
if [[ -z "$RELEASE_URL" ]]; then
  if [[ "$BEADS_WEB_VERSION" == "latest" ]]; then
    RELEASE_URL="https://github.com/${BEADS_WEB_REPO}/releases/latest/download/beads-web-linux-x64"
  else
    RELEASE_URL="https://github.com/${BEADS_WEB_REPO}/releases/download/v${BEADS_WEB_VERSION}/beads-web-linux-x64"
  fi
fi

echo ""
echo -e "${GREEN}=== beads-web Update ===${NC}"
echo ""
echo "  Install dir: $INSTALL_DIR"
echo "  Repo:        $BEADS_WEB_REPO"
echo "  Version:     $BEADS_WEB_VERSION"
echo "  Binary URL:  $RELEASE_URL"
echo ""

# ============================================
# 1. Validate release URL
# ============================================
echo -e "${YELLOW}[1/3] Checking release URL...${NC}"
if ! curl -fsSL --head "$RELEASE_URL" >/dev/null 2>&1; then
  echo -e "${RED}ERROR: Release not found at: $RELEASE_URL${NC}"
  exit 1
fi
echo "  Release URL valid"

# ============================================
# 2. Rebuild container (re-downloads binary)
# ============================================
echo -e "${YELLOW}[2/3] Rebuilding container...${NC}"
cd "$INSTALL_DIR"
docker compose build --no-cache
docker compose up -d
sleep 5

# ============================================
# 3. Verify
# ============================================
echo -e "${YELLOW}[3/3] Verifying...${NC}"
if docker compose ps --format '{{.Status}}' | grep -q "Up"; then
  echo -e "${GREEN}  beads-web running${NC}"
else
  echo -e "${RED}ERROR: beads-web failed to start. Check: docker compose logs${NC}"
  exit 1
fi

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}    beads-web Updated${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "  Logs:    docker logs beads-web"
echo "  Status:  cd $INSTALL_DIR && docker compose ps"
echo ""
echo -e "${GREEN}========================================${NC}"