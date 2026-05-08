package initcmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// PromptPrefs runs the interactive form. Returns user-selected Prefs.
// `defaults` provides initial values (from flags or detected state).
//
// The form ends with a Confirm field ("Apply" / "Cancel"). If the user picks
// Cancel (or aborts via Ctrl-C / Esc), returns ErrUserAborted - the cobra
// wrapper checks for it via errors.Is and prints "Aborted." instead of
// treating it as a real error.
func PromptPrefs(defaults Prefs) (Prefs, error) {
	prefs := defaults
	if prefs.Language == "" {
		prefs.Language = "English"
	}
	if prefs.MergeBranch == "" {
		prefs.MergeBranch = "main"
	}
	if prefs.PRFlow == "" {
		prefs.PRFlow = "local"
	}

	var customLang string
	confirm := true
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Response language").
				Description("How should I respond by default in this project?").
				Options(
					huh.NewOption("English", "English"),
					huh.NewOption("Українська", "Ukrainian"),
					huh.NewOption("Italiano", "Italian"),
					huh.NewOption("Other (type below)", "__custom__"),
				).
				Value(&prefs.Language),

			huh.NewInput().
				Title("Custom language").
				Description("Only if you picked Other above").
				Value(&customLang),

			huh.NewInput().
				Title("Merge branch").
				Description("Target branch for merges and PRs").
				Value(&prefs.MergeBranch).
				Validate(func(s string) error {
					if strings.TrimSpace(s) == "" {
						return fmt.Errorf("branch name required")
					}
					return nil
				}),

			huh.NewSelect[string]().
				Title("PR flow").
				Description("How does this project handle merges?").
				Options(
					huh.NewOption("local (no remote PR)", "local"),
					huh.NewOption("github (gh CLI)", "github"),
					huh.NewOption("bitbucket", "bitbucket"),
				).
				Value(&prefs.PRFlow),

			huh.NewConfirm().
				Title("Apply these settings?").
				Description("Run lets init with the choices above. (Pick Cancel to abort.)").
				Affirmative("Apply").
				Negative("Cancel").
				Value(&confirm),
		),
	)

	if err := form.Run(); err != nil {
		// huh returns its own ErrUserAborted on Ctrl-C / Esc.
		// Either way, surface as initcmd.ErrUserAborted.
		if errors.Is(err, huh.ErrUserAborted) {
			return Prefs{}, ErrUserAborted
		}
		return Prefs{}, err
	}

	if !confirm {
		return Prefs{}, ErrUserAborted
	}

	if prefs.Language == "__custom__" && customLang != "" {
		prefs.Language = customLang
	}

	return prefs, nil
}

// RenderWelcome prints the welcome box.
func RenderWelcome(projectRoot, pluginRoot, cliVersion string) string {
	body := fmt.Sprintf("%s\n\n  Project: %s\n  Plugin:  %s\n  CLI:     v%s",
		titleStyle.Render("🌱 LETS Workflow Setup"),
		subtitleStyle.Render(projectRoot),
		subtitleStyle.Render(pluginRoot),
		subtitleStyle.Render(cliVersion))
	return boxStyle.Render(body)
}

// RenderCompletion prints the success box with summary + next step hint.
func RenderCompletion(projectRoot string, prefs Prefs, beadsOK bool) string {
	beadsLine := "✓ initialized"
	if !beadsOK {
		beadsLine = "skipped"
	}
	body := fmt.Sprintf(
		"%s\n\n  %s   %s\n  %s  %s\n  %s     %s\n  %s   %s\n  %s     %s\n\n  %s     %s",
		titleStyle.Render("✓ LETS Workflow ready"),
		labelStyle.Render("Project:"), projectRoot,
		labelStyle.Render("Language:"), prefs.Language,
		labelStyle.Render("Merge:"), prefs.MergeBranch,
		labelStyle.Render("PR flow:"), prefs.PRFlow,
		labelStyle.Render("Beads:"), beadsLine,
		labelStyle.Render("Next:"), "/lets:start",
	)
	return boxStyle.Render(body)
}
