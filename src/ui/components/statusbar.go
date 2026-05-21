package components

import (
	"flux/src/config"
	"flux/src/utils"
	"flux/src/workspace"
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type StatusBar struct {
	Workspace    string
	Model        string
	TokensUsed   int
	TotalTokens  int
	ContextUsed  string
	Branch       string
	Status       Status
	Spinner      spinner.Model
	Mode         Mode
	InputTokens  int
	OutputTokens int
}

type Mode string

const (
	Mode_PLAN Mode = "Plan"
	Mode_EDIT Mode = "Edit"
)

type Status string

const (
	Status_IDLE    Status = "Idle"
	Status_RUNNING Status = "Running"
	Status_ERROR   Status = "Error"
)

func CreateStatusBar(workspace string, model string) StatusBar {
	return StatusBar{
		Workspace: workspace,
		Model:     model,
		Status:    Status_IDLE,
		Mode:      Mode_EDIT,
		Spinner:   spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
}

func (s StatusBar) Init() tea.Cmd {
	return s.Spinner.Tick
}

func (s *StatusBar) SetModel(model string) {
	s.Model = model
}

func (s StatusBar) Update(msg tea.Msg) (StatusBar, tea.Cmd) {
	var cmd tea.Cmd
	s.Spinner, cmd = s.Spinner.Update(msg)
	return s, cmd
}

func (s *StatusBar) UpdateUsage(input int, output int) {
	s.InputTokens = input
	s.OutputTokens = output
}

func (s StatusBar) View() string {
	width, _, _ := utils.GetTerminalSize()
	bar := renderStatusBar(width, s)

	return fmt.Sprintf("%s", bar)
}

func (s StatusBar) GetMode() Mode {
	return s.Mode
}

func (s *StatusBar) SetMode(mode Mode) {
	s.Mode = mode
}

func (s StatusBar) convertTokenValues() (string, string) {
	input := ""
	output := ""

	if s.InputTokens > 1000 {
		input = fmt.Sprintf("%.1fk", float32(s.InputTokens/1000))
	} else {
		input = fmt.Sprintf("%d", s.InputTokens)
	}

	if s.OutputTokens > 1000 {
		output = fmt.Sprintf("%.1fk", float64(s.OutputTokens/1000))
	} else {
		output = fmt.Sprintf("%d", s.OutputTokens)
	}

	return input, output
}

func renderStatusBar(width int, s StatusBar) string {
	appVersion := fmt.Sprintf("flux v%s ", config.Cfg.AppVersion)

	spinnerView := ""

	if s.Status == Status_RUNNING {
		spinnerView = s.Spinner.View()
	}

	inputTokens, outputTokens := s.convertTokenValues()

	status := fmt.Sprintf(
		" %s%s (%s) | %s (main) | %s | Usage: %s ↑ / %s ↓",
		spinnerView,
		s.Status,
		s.Mode,
		workspace.AbsToTildePath(s.Workspace),
		s.Model,
		inputTokens,
		outputTokens,
	)

	content := status + utils.FlexGap(
		width,
		len(status)+len(appVersion),
	) + appVersion

	return lipgloss.NewStyle().
		Width(width).
		Background(lipgloss.Color("#144492")).
		Foreground(lipgloss.Color("#d9d9d9")).
		Render(content)
}

func (s *StatusBar) SetStatus(value Status) {
	s.Status = value
}
