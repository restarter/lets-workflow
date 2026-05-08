package initcmd

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// RunWithSpinner shows an animated spinner during Run() execution.
// Returns Run's results unchanged. After Run() completes, the spinner exits;
// the caller is expected to render the Step list from the return value.
//
// Falls back to direct Run() (no UI) when stdout is not a TTY - decision made
// at the cobra layer (Task 12) which gates this call.
func RunWithSpinner(ctx context.Context, prefs Prefs, opts RunOptions, projectRoot, pluginRoot string) ([]Step, error) {
	doneChan := make(chan runResult, 1)

	go func() {
		steps, err := Run(ctx, prefs, opts, projectRoot, pluginRoot)
		doneChan <- runResult{steps: steps, err: err}
	}()

	m := initModel{
		spinner: spinner.New(spinner.WithSpinner(spinner.Dot)),
		done:    doneChan,
	}
	finalModel, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, err
	}
	res := finalModel.(initModel).result
	return res.steps, res.err
}

type runResult struct {
	steps []Step
	err   error
}

type runDoneMsg runResult

type initModel struct {
	spinner spinner.Model
	done    <-chan runResult
	result  runResult
}

func (m initModel) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, waitForRun(m.done))
}

func waitForRun(ch <-chan runResult) tea.Cmd {
	return func() tea.Msg {
		r := <-ch
		return runDoneMsg(r)
	}
}

func (m initModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case runDoneMsg:
		m.result = runResult(msg)
		return m, tea.Quit
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m initModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Setting up..."))
	b.WriteString("\n\n  ")
	b.WriteString(m.spinner.View())
	b.WriteString(" working...\n")
	return b.String()
}
