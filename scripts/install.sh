#!/usr/bin/env bash
#
# lets CLI installation script
# Usage: curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash
#
# IMPORTANT: must be EXECUTED, never SOURCED (set -e would kill the user's shell on errors).
#

set -e

REPO="restarter/lets-workflow"
BIN_NAME="lets"

# Colors + styles (skip if stderr isn't a TTY — keeps logs clean in CI)
if [ -t 2 ]; then
    RED='\033[0;31m'
    GREEN='\033[0;32m'
    YELLOW='\033[1;33m'
    CYAN='\033[0;36m'
    BOLD='\033[1m'
    DIM='\033[2m'
    NC='\033[0m'
else
    RED=''; GREEN=''; YELLOW=''; CYAN=''; BOLD=''; DIM=''; NC=''
fi

# Step counter — tracks user's progress through the install phases.
STEP_TOTAL=6
STEP_CURRENT=0

# print_banner: ASCII LETS logo + tagline. Skipped on non-TTY (clean CI output).
print_banner() {
    [ -t 2 ] || return 0
    printf '%b\n' "
${GREEN}   ██╗     ███████╗████████╗███████╗
   ██║     ██╔════╝╚══██╔══╝██╔════╝
   ██║     █████╗     ██║   ███████╗
   ██║     ██╔══╝     ██║   ╚════██║
   ███████╗███████╗   ██║   ███████║
   ╚══════╝╚══════╝   ╚═╝   ╚══════╝${NC}

   🌱 ${BOLD}CLI installer${NC}
   ${DIM}https://github.com/restarter/lets-workflow${NC}
" >&2
}

# step: emit a numbered phase header. Resets the visual rhythm of the script.
step() {
    STEP_CURRENT=$((STEP_CURRENT + 1))
    echo "" >&2
    printf '%b\n' "${CYAN}[${STEP_CURRENT}/${STEP_TOTAL}]${NC} ${BOLD}$1${NC}" >&2
}

# All logs go to stderr — keeps stdout clean for any future scripted use.
# Icons: · neutral info, ✓ success, ⚠ warning, ✗ error.
log_info()    { printf '%b\n' "  ${DIM}·${NC} $1" >&2; }
log_success() { printf '%b\n' "  ${GREEN}✓${NC} $1" >&2; }
log_warning() { printf '%b\n' "  ${YELLOW}⚠${NC} $1" >&2; }
log_error()   { printf '%b\n' "  ${RED}✗ Error:${NC} $1" >&2; }

# detect_platform: prints "<os>_<arch>" or fails with explicit error.
detect_platform() {
    local os arch
    case "$(uname -s)" in
        MINGW*|MSYS*|CYGWIN*)
            log_error "Windows detected ($(uname -s))."
            echo "" >&2
            echo "  This bash installer is for macOS/Linux only." >&2
            echo "  Native Windows install (winget/scoop/install.ps1) is tracked under lets-hdrdr.1." >&2
            echo "  Until then, install via WSL or Git Bash with care." >&2
            echo "" >&2
            exit 1
            ;;
        Darwin) os="darwin" ;;
        Linux)
            if [ -f /proc/version ] && grep -qi 'microsoft\|wsl' /proc/version 2>/dev/null; then
                log_warning "WSL detected. Installing the Linux build (visible only inside WSL)."
            fi
            os="linux"
            ;;
        *)
            log_error "Unsupported OS: $(uname -s)"
            echo "  Supported: Darwin (macOS), Linux." >&2
            exit 1
            ;;
    esac
    case "$(uname -m)" in
        x86_64|amd64) arch="amd64" ;;
        aarch64|arm64) arch="arm64" ;;
        *)
            log_error "Unsupported architecture: $(uname -m)"
            echo "  Supported: amd64 (x86_64), arm64 (aarch64)." >&2
            exit 1
            ;;
    esac
    echo "${os}_${arch}"
}

# auth_header_curl / auth_header_wget: emit Authorization arg-array when GITHUB_TOKEN is set.
# Useful for: (a) private repos during dev, (b) CI environments hitting the api.github.com
# unauthenticated rate limit (60/hr per IP), (c) GitHub Enterprise installs.
# Empty when token unset — falls back to anonymous requests.
auth_header_curl() {
    if [ -n "${GITHUB_TOKEN:-}" ]; then
        printf '%s\n' "-H" "Authorization: Bearer ${GITHUB_TOKEN}"
    fi
}
auth_header_wget() {
    if [ -n "${GITHUB_TOKEN:-}" ]; then
        printf '%s\n' "--header=Authorization: Bearer ${GITHUB_TOKEN}"
    fi
}

# fetch_release_json: queries /releases/latest, sets RELEASE_JSON.
fetch_release_json() {
    local url="https://api.github.com/repos/${REPO}/releases/latest"
    if command -v curl &>/dev/null; then
        local auth_args=()
        while IFS= read -r line; do [ -n "$line" ] && auth_args+=("$line"); done < <(auth_header_curl)
        RELEASE_JSON=$(curl -fsSL "${auth_args[@]}" "$url")
    elif command -v wget &>/dev/null; then
        local auth_args=()
        while IFS= read -r line; do [ -n "$line" ] && auth_args+=("$line"); done < <(auth_header_wget)
        RELEASE_JSON=$(wget -qO- "${auth_args[@]}" "$url")
    else
        log_error "Neither curl nor wget found. Install one and retry."
        exit 1
    fi
}

# extract_tag_name: pulls "tag_name" from RELEASE_JSON (e.g. v0.5.0).
extract_tag_name() {
    echo "$RELEASE_JSON" | grep '"tag_name"' | sed -E 's/.*"tag_name": "([^"]+)".*/\1/'
}

# release_has_asset: returns 0 if RELEASE_JSON contains an asset with the given name.
release_has_asset() {
    local name=$1
    echo "$RELEASE_JSON" | grep -Fq "\"name\": \"$name\""
}

# list_release_assets: prints "  - <name>" lines for each asset (used in error messages).
list_release_assets() {
    echo "$RELEASE_JSON" | grep '"name"' | sed -E 's/.*"name": "([^"]+)".*/  - \1/'
}

# extract_asset_url: given asset_name, prints the API URL for that asset.
# Used for the GITHUB_TOKEN code path — API asset URLs work for both private
# repos (with auth) and public repos. Public flow uses github.com download URLs
# directly to avoid api.github.com rate limit (60/hr unauth).
extract_asset_url() {
    local target=$1
    echo "$RELEASE_JSON" | awk -v target="$target" '
        /^[[:space:]]*"url":/ {
            line = $0
            sub(/^[[:space:]]*"url":[[:space:]]*"/, "", line)
            sub(/".*$/, "", line)
            if (line ~ /\/releases\/assets\//) { last_url = line }
            next
        }
        /^[[:space:]]*"name":/ {
            line = $0
            sub(/^[[:space:]]*"name":[[:space:]]*"/, "", line)
            sub(/".*$/, "", line)
            if (line == target && last_url != "") { print last_url; exit }
        }
    '
}

# download_file: url, output_path. Uses curl or wget. Returns non-zero on HTTP error.
# Honors GITHUB_TOKEN. Always sends Accept: application/octet-stream so API asset URLs
# (api.github.com/repos/.../releases/assets/<id>) redirect to the binary; harmless for
# public github.com/.../releases/download/... URLs which ignore the header.
download_file() {
    local url=$1 out=$2
    if command -v curl &>/dev/null; then
        local auth_args=()
        while IFS= read -r line; do [ -n "$line" ] && auth_args+=("$line"); done < <(auth_header_curl)
        curl -fsSL "${auth_args[@]}" -H "Accept: application/octet-stream" -o "$out" "$url"
    elif command -v wget &>/dev/null; then
        local auth_args=()
        while IFS= read -r line; do [ -n "$line" ] && auth_args+=("$line"); done < <(auth_header_wget)
        wget -q "${auth_args[@]}" --header="Accept: application/octet-stream" -O "$out" "$url"
    else
        log_error "Neither curl nor wget found."
        return 1
    fi
}

# sha256_file: prints lowercase hex digest of <file>. Tries sha256sum, shasum -a 256, openssl in order.
sha256_file() {
    local f=$1
    if command -v sha256sum &>/dev/null; then
        sha256sum "$f" | awk '{print $1}'
    elif command -v shasum &>/dev/null; then
        shasum -a 256 "$f" | awk '{print $1}'
    elif command -v openssl &>/dev/null; then
        openssl dgst -sha256 "$f" | awk '{print $2}'
    else
        return 1
    fi
}

# verify_checksum: archive_name, archive_path, checksums_path.
# Looks up archive_name in checksums_path (goreleaser format: "<sha>  <name>" or "<sha>  *<name>").
# Returns 0 on match, non-zero on mismatch / missing entry / no SHA tool.
verify_checksum() {
    local archive_name=$1 archive_path=$2 checksums_path=$3
    local expected actual
    expected=$(awk -v target="$archive_name" '
        {name=$2; sub(/^\*/, "", name); if (name == target) {print $1; exit}}
    ' "$checksums_path")
    if [ -z "$expected" ]; then
        log_error "No checksum entry for $archive_name in $(basename "$checksums_path")."
        return 1
    fi
    actual=$(sha256_file "$archive_path") || {
        log_error "No SHA256 tool available (need sha256sum, shasum, or openssl)."
        return 1
    }
    if [ "$expected" != "$actual" ]; then
        log_error "Checksum mismatch for $archive_name."
        log_error "  Expected: $expected"
        log_error "  Actual:   $actual"
        return 1
    fi
    log_success "Checksum verified: $archive_name"
}

# pick_install_dir: /usr/local/bin if writable, else $HOME/.local/bin (created if missing).
# We deliberately don't auto-elevate via sudo — users who want a global install can run
# `sudo bash install.sh` explicitly. Surprise sudo prompts on a curl-pipe-bash flow are bad UX.
pick_install_dir() {
    if [ -w /usr/local/bin ] 2>/dev/null; then
        echo "/usr/local/bin"
    else
        local dir="$HOME/.local/bin"
        mkdir -p "$dir"
        echo "$dir"
    fi
}

# path_contains: returns 0 if $1 appears as a $PATH entry.
path_contains() {
    case ":$PATH:" in
        *":$1:"*) return 0 ;;
        *)        return 1 ;;
    esac
}

# find_lets_on_path: prints unique resolved paths to "lets" found via $PATH (one per line).
# bash 3.2 compatible — no mapfile, no associative arrays.
find_lets_on_path() {
    local IFS=':'
    local p resolved
    local seen=""
    for p in $PATH; do
        [ -z "$p" ] && continue
        if [ -x "$p/$BIN_NAME" ]; then
            if command -v readlink &>/dev/null; then
                resolved=$(readlink -f "$p/$BIN_NAME" 2>/dev/null || printf '%s' "$p/$BIN_NAME")
            else
                resolved="$p/$BIN_NAME"
            fi
            case ":$seen:" in
                *":$resolved:"*) ;;  # duplicate, skip
                *) printf '%s\n' "$resolved"; seen="${seen}:${resolved}" ;;
            esac
        fi
    done
}

main() {
    print_banner

    step "Detecting platform"
    local platform
    platform=$(detect_platform)
    log_success "${BOLD}${platform}${NC}"

    step "Fetching latest release"
    fetch_release_json
    local tag
    tag=$(extract_tag_name)
    if [ -z "$tag" ]; then
        log_error "Failed to extract tag from GitHub API response."
        printf '%b\n' "    ${DIM}This usually means the API rate limit is exhausted or the repo has no releases yet.${NC}" >&2
        printf '%b\n' "    ${DIM}See: https://github.com/${REPO}/releases${NC}" >&2
        exit 1
    fi
    log_success "${BOLD}${tag}${NC}"

    local version="${tag#v}"
    local archive_name="lets_${version}_${platform}.tar.gz"
    local checksums_name="lets_${version}_checksums.txt"

    if ! release_has_asset "$archive_name"; then
        log_error "No prebuilt archive for $platform in release $tag."
        printf '%b\n' "    ${DIM}Expected asset: $archive_name${NC}" >&2
        printf '%b\n' "    ${DIM}Available in this release:${NC}" >&2
        list_release_assets >&2
        echo "" >&2
        printf '%b\n' "    ${DIM}Open an issue at https://github.com/${REPO}/issues if your platform should be supported.${NC}" >&2
        exit 1
    fi
    if ! release_has_asset "$checksums_name"; then
        log_error "No $checksums_name in release; refusing to install unverified binary."
        exit 1
    fi

    local tmpdir
    tmpdir=$(mktemp -d)
    # shellcheck disable=SC2064  # We want $tmpdir expanded NOW (at trap-set time),
    # not when EXIT fires — `local tmpdir` is out of scope by then.
    trap "rm -rf '$tmpdir'" EXIT

    # URL source: with token → API asset URLs (works for private repos, GitHub Enterprise);
    # without token → public github.com download URLs (no api.github.com rate-limit hit).
    local archive_url checksums_url
    if [ -n "${GITHUB_TOKEN:-}" ]; then
        archive_url=$(extract_asset_url "$archive_name")
        checksums_url=$(extract_asset_url "$checksums_name")
        if [ -z "$archive_url" ] || [ -z "$checksums_url" ]; then
            log_error "Failed to extract asset URLs from release JSON."
            exit 1
        fi
    else
        archive_url="https://github.com/${REPO}/releases/download/${tag}/${archive_name}"
        checksums_url="https://github.com/${REPO}/releases/download/${tag}/${checksums_name}"
    fi

    step "Downloading"
    log_info "${DIM}${archive_name}${NC}"
    download_file "$archive_url" "$tmpdir/$archive_name" || {
        log_error "Download failed."
        exit 1
    }
    log_info "${DIM}${checksums_name}${NC}"
    download_file "$checksums_url" "$tmpdir/$checksums_name" || {
        log_error "Failed to download checksums; refusing to install unverified binary."
        exit 1
    }
    log_success "Both artifacts fetched"

    step "Verifying SHA256"
    verify_checksum "$archive_name" "$tmpdir/$archive_name" "$tmpdir/$checksums_name" || exit 1

    step "Installing"
    log_info "Extracting archive..."
    tar -xzf "$tmpdir/$archive_name" -C "$tmpdir" || {
        log_error "Failed to extract $archive_name."
        exit 1
    }

    # Locate the binary — resilient to goreleaser `wrap_in_directory` mode.
    # `-name` filter is sufficient (archive contents are predictable); chmod +x below handles non-executable case.
    local extracted_bin
    extracted_bin=$(find "$tmpdir" -type f -name "$BIN_NAME" 2>/dev/null | head -1)
    if [ -z "$extracted_bin" ]; then
        log_error "Binary '$BIN_NAME' not found in archive after extraction."
        printf '%b\n' "    ${DIM}Archive contents:${NC}" >&2
        find "$tmpdir" -maxdepth 2 -type f >&2
        exit 1
    fi

    local install_dir
    install_dir=$(pick_install_dir)
    log_info "Moving binary to ${BOLD}${install_dir}/${BIN_NAME}${NC}..."
    mv "$extracted_bin" "$install_dir/$BIN_NAME"
    chmod +x "$install_dir/$BIN_NAME"

    log_success "Installed → ${BOLD}${install_dir}/${BIN_NAME}${NC}"

    # PATH check
    if ! path_contains "$install_dir"; then
        echo "" >&2
        log_warning "${install_dir} is not in your PATH."
        printf '%b\n' "    ${DIM}Add this to your shell profile (~/.bashrc, ~/.zshrc, ~/.profile):${NC}" >&2
        echo "" >&2
        printf '%b\n' "      ${BOLD}export PATH=\"\$PATH:${install_dir}\"${NC}" >&2
        echo "" >&2
    fi

    # Multiple-binary warning — per-binary version output + first-in-PATH vs installed comparison
    # so the user gets actionable feedback (which copy wins, what to reorder/remove).
    # bash 3.2 compatible: read into array via while loop, no mapfile.
    local lets_paths=()
    while IFS= read -r line; do
        [ -n "$line" ] && lets_paths+=("$line")
    done < <(find_lets_on_path)

    if [ "${#lets_paths[@]}" -gt 1 ]; then
        echo "" >&2
        log_warning "Multiple '$BIN_NAME' executables on your PATH — an older copy may be executed instead of the one we installed."
        printf '%b\n' "    ${DIM}Found (entries earlier in PATH take precedence):${NC}" >&2
        local i=1 p ver
        for p in "${lets_paths[@]}"; do
            ver=""
            if [ -x "$p" ]; then
                # `lets version` may print multi-line; collapse to first non-empty line for readability.
                ver=$("$p" version 2>/dev/null | head -1 || true)
            fi
            [ -z "$ver" ] && ver="<unknown version>"
            printf '%b\n' "      ${BOLD}${i}.${NC} $p  ${DIM}→${NC}  $ver" >&2
            i=$((i+1))
        done

        # Resolve install path (find_lets_on_path resolves symlinks, so compare apples-to-apples).
        local installed_resolved="$install_dir/$BIN_NAME"
        if command -v readlink &>/dev/null; then
            installed_resolved=$(readlink -f "$install_dir/$BIN_NAME" 2>/dev/null || printf '%s' "$install_dir/$BIN_NAME")
        fi

        echo "" >&2
        printf '%b\n' "    ${DIM}We installed to: ${install_dir}/${BIN_NAME}${NC}" >&2
        local first="${lets_paths[0]}"
        if [ "$first" != "$installed_resolved" ]; then
            log_warning "The '$BIN_NAME' first in your PATH is different from the one we installed."
            printf '%b\n' "    ${DIM}To make the newly installed '$BIN_NAME' the one you get when running '$BIN_NAME', either:${NC}" >&2
            printf '%b\n' "      ${BOLD}-${NC} Remove or rename the older $first from your PATH, or" >&2
            printf '%b\n' "      ${BOLD}-${NC} Reorder your PATH so $(dirname "$installed_resolved") appears before $(dirname "$first")" >&2
            printf '%b\n' "    ${DIM}After updating PATH, restart your shell and run '$BIN_NAME version' to confirm.${NC}" >&2
        else
            log_success "The installed '$BIN_NAME' is first in your PATH."
        fi
        echo "" >&2
    fi

    step "Verifying installation"
    if "$install_dir/$BIN_NAME" version >/dev/null 2>&1; then
        local v
        v=$("$install_dir/$BIN_NAME" version 2>&1 | head -1)
        log_success "${BOLD}${v}${NC}"

        # Final celebration banner — binary done, but plugin + project init still needed.
        printf '%b\n' "
   ${GREEN}🌱 lets binary ready!${NC}

   ${BOLD}▸${NC} ${DIM}Install the plugin (in Claude Code, one-time):${NC}
     ${BOLD}/plugin marketplace add restarter/lets-workflow${NC}
     ${BOLD}/plugin install lets${NC}

   ${BOLD}▸${NC} ${DIM}Initialize a project:${NC}
     ${BOLD}cd${NC} your-project ${DIM}&&${NC} ${BOLD}claude${NC}
     ${BOLD}/lets:init${NC}

   📘 ${DIM}Full guide:${NC} https://github.com/restarter/lets-workflow/blob/main/docs/installation.md
" >&2
    else
        log_warning "Binary installed but '$BIN_NAME version' returned non-zero."
        log_warning "Try: $install_dir/$BIN_NAME version"
    fi
}

main "$@"
