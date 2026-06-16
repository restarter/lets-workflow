#!/usr/bin/env bash
#
# manage-users.sh - manage scoped SQL users on the shared Dolt VPS.
#
# Runs ON the VPS (piped over ssh, same pattern as setup-remote.sh):
#   ssh proxy-master "bash -s -- --add-user alice --dbs pp_mngr,pp_dev" < scripts/remote/dolt/manage-users.sh
#
# Why a throwaway client container (not `docker exec dolt dolt sql`):
#   Users created via the dolt CLI are INVISIBLE to the running SQL server.
#   We must talk to the server over the MySQL protocol. There is no mysql
#   client in the dolt container, so we run a one-shot client on `dolt-net`
#   that reaches the server at `dolt:3306` - no host package, no IP-allowlist
#   involvement (internal docker network).
#
# Dolt has NO REVOKE: GRANT is additive (--grant keeps the password), but the
# only way to REMOVE a db from a user is DROP+CREATE (--set-dbs), which ROTATES
# the password (printed once).
#
# Subcommands:
#   --add-user <name>   --dbs a,b   create user (generated password) + grant the dbs
#   --grant <name>      --dbs a,b   additive GRANT only (password unchanged)
#   --set-dbs <name>    --dbs a,b   authoritative: DROP+CREATE, grant exactly these dbs (rotates password)
#   --remove-user <name>            drop the user
#   --list-users                    list users
#   --list-grants <name>            show grants for a user

set -euo pipefail

RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'; NC='\033[0m'

INSTALL_DIR="${INSTALL_DIR:-/opt/dolt-remote}"
CLIENT_IMAGE="${CLIENT_IMAGE:-mysql:8}"
NETWORK="${NETWORK:-dolt-net}"
DB_CONTAINER="${DB_CONTAINER:-dolt}"

die() { echo -e "${RED}Error: $*${NC}" >&2; exit 1; }

validate_name() {
  local value="$1" label="$2"
  [[ "$value" =~ ^[a-zA-Z0-9_-]+$ ]] || die "${label} must be alphanumeric (a-z, 0-9, _, -)"
}

# Load the server root password from the deployment .env.
load_root_password() {
  local env_file="$INSTALL_DIR/.env"
  [ -f "$env_file" ] || die "$env_file not found - is this the Dolt VPS?"
  ROOT_PASSWORD=$(grep '^ROOT_PASSWORD=' "$env_file" | cut -d= -f2-)
  [ -n "$ROOT_PASSWORD" ] || die "ROOT_PASSWORD missing from $env_file"
}

# Run SQL against the running server via a throwaway client on dolt-net.
run_sql() {
  docker run --rm --network "$NETWORK" "$CLIENT_IMAGE" \
    mysql -h "$DB_CONTAINER" -P 3306 -uroot -p"$ROOT_PASSWORD" \
    --default-auth=mysql_native_password -N -e "$1"
}

gen_password() { openssl rand -hex 16; }

# Build "GRANT ALL ON `db`.* TO `name`@`%`;" for each comma-separated db.
grant_stmts() {
  local name="$1" dbs="$2" out="" db
  IFS=',' read -ra _dbs <<< "$dbs"
  for db in "${_dbs[@]}"; do
    [ -z "$db" ] && continue
    validate_name "$db" "Database name '$db'"
    out+="GRANT ALL ON \`${db}\`.* TO \`${name}\`@\`%\`; "
  done
  [ -n "$out" ] || die "no databases given (use --dbs a,b)"
  printf '%s' "$out"
}

cmd_add_user() {
  local name="$1" dbs="$2"
  validate_name "$name" "User name '$name'"
  [ -n "$dbs" ] || die "--add-user needs --dbs a,b"
  local pw; pw=$(gen_password)
  run_sql "CREATE USER \`${name}\`@\`%\` IDENTIFIED BY '${pw}'; $(grant_stmts "$name" "$dbs") FLUSH PRIVILEGES;"
  echo -e "${GREEN}✓ User '${name}' created${NC}"
  echo "  Databases: ${dbs}"
  echo "  Username:  ${name}"
  echo "  Password:  ${pw}"
  echo -e "${YELLOW}  Save the password now - it is not stored anywhere.${NC}"
}

cmd_grant() {
  local name="$1" dbs="$2"
  validate_name "$name" "User name '$name'"
  [ -n "$dbs" ] || die "--grant needs --dbs a,b"
  run_sql "$(grant_stmts "$name" "$dbs") FLUSH PRIVILEGES;"
  echo -e "${GREEN}✓ Granted ${dbs} to '${name}' (password unchanged)${NC}"
}

cmd_set_dbs() {
  local name="$1" dbs="$2"
  validate_name "$name" "User name '$name'"
  [ -n "$dbs" ] || die "--set-dbs needs --dbs a,b"
  local pw; pw=$(gen_password)
  # Dolt has no REVOKE - drop+recreate is the only way to REMOVE a db. Rotates the password.
  run_sql "DROP USER IF EXISTS \`${name}\`@\`%\`; CREATE USER \`${name}\`@\`%\` IDENTIFIED BY '${pw}'; $(grant_stmts "$name" "$dbs") FLUSH PRIVILEGES;"
  echo -e "${GREEN}✓ User '${name}' set to exactly: ${dbs}${NC}"
  echo "  Username:  ${name}"
  echo "  Password:  ${pw}  (ROTATED - re-distribute)"
  echo -e "${YELLOW}  set-dbs rotates the password because Dolt has no REVOKE.${NC}"
}

cmd_remove_user() {
  local name="$1"
  validate_name "$name" "User name '$name'"
  run_sql "DROP USER IF EXISTS \`${name}\`@\`%\`; FLUSH PRIVILEGES;"
  echo -e "${GREEN}✓ User '${name}' removed${NC}"
}

cmd_list_users() {
  echo -e "${YELLOW}Users:${NC}"
  run_sql "SELECT User, Host FROM mysql.user ORDER BY User;"
}

cmd_list_grants() {
  local name="$1"
  validate_name "$name" "User name '$name'"
  echo -e "${YELLOW}Grants for '${name}':${NC}"
  run_sql "SHOW GRANTS FOR \`${name}\`@\`%\`;"
}

usage() {
  cat <<EOF
manage-users.sh - scoped SQL users on the Dolt VPS

  --add-user <name> --dbs a,b    create user + grant dbs (prints generated password)
  --grant <name> --dbs a,b       additive grant (password unchanged)
  --set-dbs <name> --dbs a,b     authoritative set (DROP+CREATE, rotates password)
  --remove-user <name>           drop user
  --list-users                   list users
  --list-grants <name>           show grants for a user
EOF
  exit 1
}

# ---- arg parsing ----
ACTION=""; NAME=""; DBS=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --add-user|--grant|--set-dbs|--remove-user|--list-grants)
      ACTION="$1"; NAME="${2:-}"; shift 2 ;;
    --list-users) ACTION="$1"; shift ;;
    --dbs) DBS="${2:-}"; shift 2 ;;
    -h|--help) usage ;;
    *) die "unknown argument: $1 (see --help)" ;;
  esac
done

[ -n "$ACTION" ] || usage
command -v docker >/dev/null 2>&1 || die "docker not found"
load_root_password

case "$ACTION" in
  --add-user)    [ -n "$NAME" ] || die "--add-user needs a name";    cmd_add_user "$NAME" "$DBS" ;;
  --grant)       [ -n "$NAME" ] || die "--grant needs a name";       cmd_grant "$NAME" "$DBS" ;;
  --set-dbs)     [ -n "$NAME" ] || die "--set-dbs needs a name";     cmd_set_dbs "$NAME" "$DBS" ;;
  --remove-user) [ -n "$NAME" ] || die "--remove-user needs a name"; cmd_remove_user "$NAME" ;;
  --list-users)  cmd_list_users ;;
  --list-grants) [ -n "$NAME" ] || die "--list-grants needs a name"; cmd_list_grants "$NAME" ;;
  *) usage ;;
esac
