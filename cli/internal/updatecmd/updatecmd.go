package updatecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/semver"

	"github.com/restarter/lets-workflow/cli/internal/drift"
	"github.com/restarter/lets-workflow/cli/internal/frontmatter"
	"github.com/restarter/lets-workflow/cli/internal/initcmd"
	"github.com/restarter/lets-workflow/cli/internal/letsconfig"
	"github.com/restarter/lets-workflow/cli/internal/version"
)

const (
	binaryUpdateAction = "Update the lets binary: `curl -fsSL https://raw.githubusercontent.com/restarter/lets-workflow/main/scripts/install.sh | bash`"
	pluginUpdateAction = "Update the plugin - easiest, do this once: in `/plugin` -> Marketplaces -> lets-workflow -> Enable auto-update (it then updates itself on startup). To update by hand instead: `/plugin marketplace update lets-workflow`, then `claude plugin update lets@lets-workflow --scope project` (use the scope you installed at - `project` is what we recommend, but it may be `user` or `local`; `claude plugin list` shows it)"
)

// Options carries injectable dependencies. LatestFn resolves the latest stable
// release version; nil means "skip network checks" (--offline). The cobra
// wrapper sets it to a closure over FetchLatest; tests pass a stub.
type Options struct {
	LatestFn func(context.Context) (LatestInfo, error)
}

// Run checks the four drift-able artifacts, auto-syncing .env (header refresh
// when LETS_ENV_VERSION is stale) and the rules file (re-copy when drift is
// detected), and reporting version status for the lets binary and the Claude
// Code plugin (which it cannot self-update). Returns a fully populated Result;
// error is reserved for hard failures (write errors).
//
// Run does NOT prompt for config, touch settings.json, or run beads - that's
// `lets init`'s job. `lets update` only syncs version-pinned artifacts.
func Run(ctx context.Context, opts Options, projectRoot, pluginRoot string) (Result, error) {
	result := NewResult(projectRoot, pluginRoot)

	// Resolve "latest" once, shared by the binary and plugin checks.
	offline := opts.LatestFn == nil
	var latest LatestInfo
	var latestErr error
	if !offline {
		latest, latestErr = opts.LatestFn(ctx)
	}

	// --- Artifact 1: .lets/.env ---
	envPath := filepath.Join(projectRoot, ".lets", ".env")
	_, envStatErr := os.Stat(envPath)
	switch {
	case os.IsNotExist(envStatErr):
		result.Add(Artifact{Name: ".env", Status: StatusNotInitialized, Action: "Run /lets:init"})
	case version.IsDev():
		// A dev binary has no real version to stamp; RegenerateEnv would rewrite
		// LETS_ENV_VERSION to "dev" (a confusing downgrade). Skip it - same
		// short-circuit the binary artifact below uses.
		result.Add(Artifact{Name: ".env", Status: StatusDev, Detail: "untagged dev build - .env regen skipped"})
	default:
		action, err := initcmd.RegenerateEnv(envPath, initcmd.Prefs{Tracker: letsconfig.Defaults()["LETS_TRACKER"]})
		if err != nil {
			return result, fmt.Errorf("regenerate .env: %w", err)
		}
		switch action.Kind {
		case initcmd.EnvSkip:
			result.Add(Artifact{Name: ".env", Status: StatusUpToDate, CurrentVersion: action.PrevVersion})
		case initcmd.EnvRegenerated:
			a := Artifact{Name: ".env", Status: StatusUpdated, CurrentVersion: action.NewVersion}
			if len(action.ChangedKeys) > 0 {
				a.Detail = fmt.Sprintf("was %s; %d key(s) changed; backup %s", version.Format(action.PrevVersion), len(action.ChangedKeys), action.BackupPath)
			} else {
				a.Detail = fmt.Sprintf("was %s; header refreshed; backup %s", version.Format(action.PrevVersion), action.BackupPath)
			}
			result.Add(a)
		case initcmd.EnvCreated:
			// Unreachable: the os.Stat above already routed a missing file to
			// StatusNotInitialized. Kept for switch completeness.
			result.Add(Artifact{Name: ".env", Status: StatusUpdated, CurrentVersion: action.NewVersion, Detail: "created"})
		}
	}

	// --- Artifact 2: .claude/rules/lets-rules.md ---
	rulesSrc := filepath.Join(pluginRoot, "rules", "lets-rules.md")
	rulesDst := filepath.Join(projectRoot, ".claude", "rules", "lets-rules.md")
	if rulesData, readErr := os.ReadFile(rulesSrc); readErr != nil {
		result.Add(Artifact{Name: "rules", Status: StatusUnknown, Detail: fmt.Sprintf("plugin rules unreadable: %s", rulesSrc)})
	} else {
		dr := drift.Check(rulesSrc, rulesDst)
		switch {
		case dr.State == drift.StatePluginUnreadable:
			// rulesSrc exists and is readable (the os.ReadFile above succeeded)
			// but has no parseable `version:` frontmatter - "couldn't check",
			// not "up to date".
			result.Add(Artifact{Name: "rules", Status: StatusUnknown, Detail: "plugin rules version unparseable (no `version:` frontmatter)"})
		case dr.Detected():
			if err := os.MkdirAll(filepath.Dir(rulesDst), 0o755); err != nil {
				return result, err
			}
			if err := initcmd.AtomicWriteBytes(rulesDst, rulesData, 0o644); err != nil {
				return result, fmt.Errorf("write rules: %w", err)
			}
			drPost := drift.Check(rulesSrc, rulesDst)
			result.Add(Artifact{Name: "rules", Status: StatusUpdated, CurrentVersion: drPost.InstalledVersion, Detail: drift.Message(dr)})
		default:
			result.Add(Artifact{Name: "rules", Status: StatusUpToDate, CurrentVersion: dr.InstalledVersion})
		}
	}

	// --- Artifact 3: lets binary ---
	result.Add(versionArtifact("binary", version.Version, latest, latestErr, offline, binaryUpdateAction))

	// --- Artifact 4: Claude Code plugin ---
	pluginVer := ReadPluginVersion(pluginRoot)
	result.Add(versionArtifact("plugin", pluginVer, latest, latestErr, offline, pluginUpdateAction))

	// --- internal consistency (binary == plugin == installed-rules frontmatter) ---
	result.Consistent = consistentVersions(version.Version, pluginVer, frontmatter.ReadVersion(rulesDst))

	return result, nil
}

// versionArtifact builds an Artifact for a version-only artifact (binary, plugin).
func versionArtifact(name, current string, latest LatestInfo, latestErr error, offline bool, action string) Artifact {
	a := Artifact{Name: name, CurrentVersion: current}
	switch current {
	case "":
		a.Status = StatusUnknown
		a.Detail = "could not determine installed version"
		return a
	case "dev":
		a.Status = StatusDev
		a.Detail = "untagged dev build - no release comparison"
		return a
	}
	if offline {
		a.Status = StatusUnknown
		a.Detail = "skipped (--offline)"
		return a
	}
	if latestErr != nil || latest.Version == "" {
		a.Status = StatusUnknown
		a.Detail = "could not check the latest release on github.com (offline, rate-limited, or no published release)"
		return a
	}
	a.LatestVersion = latest.Version
	if latest.Source == "cache" {
		a.Detail = fmt.Sprintf("latest checked %s ago", durationApprox(time.Since(latest.CheckedAt)))
	}
	switch semver.Compare("v"+current, "v"+latest.Version) {
	case 0:
		a.Status = StatusUpToDate
	case -1:
		a.Status = StatusOutdated
		a.Action = action
	case 1:
		a.Status = StatusAhead
		if a.Detail != "" {
			a.Detail += "; "
		}
		a.Detail += "newer than the latest stable release (prerelease or local checkout)"
	}
	return a
}

// consistentVersions reports whether all non-empty, non-"dev" version strings
// among the args are mutually equal.
func consistentVersions(vs ...string) bool {
	var seen string
	for _, v := range vs {
		if v == "" || v == "dev" {
			continue
		}
		if seen == "" {
			seen = v
			continue
		}
		if v != seen {
			return false
		}
	}
	return true
}

// durationApprox renders a coarse human duration ("3m", "2h", "5d"). Sub-minute
// and negative inputs (clock skew) collapse to "<1m".
func durationApprox(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

// PrintReport writes a human-readable status table to w.
func PrintReport(w io.Writer, r Result) {
	fmt.Fprintln(w, "LETS Update Status")
	fmt.Fprintln(w, "==================")
	for _, a := range r.Artifacts {
		ver := "?"
		if a.CurrentVersion != "" {
			ver = version.Format(a.CurrentVersion)
		}
		line := fmt.Sprintf("%-8s %-9s %s", a.Name, ver, a.Status)
		if a.LatestVersion != "" {
			line += fmt.Sprintf(" (latest %s)", version.Format(a.LatestVersion))
		}
		if a.Detail != "" {
			line += " - " + a.Detail
		}
		fmt.Fprintln(w, line)
	}
	fmt.Fprintln(w)
	if !r.Consistent {
		fmt.Fprintln(w, "warning: local install is inconsistent (binary / plugin / rules versions differ) - likely a partial upgrade")
	}
	if r.Summary.ActionNeeded == 0 {
		if r.Summary.Unknown > 0 {
			fmt.Fprintf(w, "%d artifact(s) in sync, %d couldn't be checked; nothing needs your action.\n", r.Summary.UpToDate+r.Summary.Updated, r.Summary.Unknown)
		} else {
			fmt.Fprintf(w, "All %d artifacts in sync.\n", len(r.Artifacts))
		}
		return
	}
	fmt.Fprintf(w, "%d of %d artifacts need your action:\n", r.Summary.ActionNeeded, len(r.Artifacts))
	for _, a := range r.Artifacts {
		if a.Action != "" {
			fmt.Fprintf(w, "  - %s: %s\n", a.Name, a.Action)
		}
	}
}
