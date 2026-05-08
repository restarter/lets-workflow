package initcmd

import "github.com/charmbracelet/lipgloss"

// LETS-themed lipgloss styles. Colors mirror plugins/lets/scripts/lets/statusline.sh.
var (
	colorGold    = lipgloss.Color("#FFD700") // bold gold (LETS Workflow)
	colorGoldDim = lipgloss.Color("#997A00") // separator
	colorPeach   = lipgloss.Color("#E8A090") // branch / migrate
	colorOrange  = lipgloss.Color("#FFAF32") // model/title
	colorTan     = lipgloss.Color("#BEB08C") // window
	colorGreen   = lipgloss.Color("#82C882")
	colorYellow  = lipgloss.Color("#FFC850")
	colorRed     = lipgloss.Color("#FF6464")
	colorGrayDim = lipgloss.Color("#666666")

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorGold)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(colorTan)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorGoldDim).
			Padding(0, 2)

	labelStyle = lipgloss.NewStyle().
			Foreground(colorOrange)

	stepOkStyle      = lipgloss.NewStyle().Foreground(colorGreen)
	stepSkipStyle    = lipgloss.NewStyle().Foreground(colorGrayDim)
	stepWarnStyle    = lipgloss.NewStyle().Foreground(colorYellow)
	stepErrStyle     = lipgloss.NewStyle().Foreground(colorRed)
	stepMigrateStyle = lipgloss.NewStyle().Foreground(colorPeach)
)

// renderStepLine produces a single line for the apply progress.
func renderStepLine(s Step) string {
	var icon string
	var style lipgloss.Style
	switch s.Status {
	case StepOK:
		icon = "✓"
		style = stepOkStyle
	case StepSkip:
		icon = "·"
		style = stepSkipStyle
	case StepWarn:
		icon = "!"
		style = stepWarnStyle
	case StepErr:
		icon = "✗"
		style = stepErrStyle
	case StepMigrate:
		icon = "~"
		style = stepMigrateStyle
	default:
		icon = "?"
		style = lipgloss.NewStyle()
	}
	return style.Render(icon+" ") + s.Message
}
