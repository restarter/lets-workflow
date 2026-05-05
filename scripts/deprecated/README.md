# scripts/deprecated/

Tools no longer used. Kept for two reasons:

1. **Historical reference** - implementation details and design decisions for review.
2. **Decommission runbooks** - operators still running these on a VPS need cleanup steps in the subdirectory READMEs.

Each subdir's README opens with a `DEPRECATED` banner, points to the replacement, and documents VPS cleanup if applicable.

**Do not deploy from here.** Use the active tools in `scripts/`.

## Current contents

| Subdir | Replaced by | Reason |
|---|---|---|
| `beads-ui/` | `scripts/beads-web/` | Node.js dashboard (port 9080) superseded by Rust/Axum binary (port 3008) - faster, broader feature set |
| `beads/` | Direct SQL via `scripts/dolt/setup-remote.sh` | Legacy push/pull mode (remotesapi); Direct SQL is the recommended deployment now |
