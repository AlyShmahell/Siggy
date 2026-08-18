package tui

import "github.com/charmbracelet/lipgloss"

var (
	colBg     = lipgloss.Color("#0e0f11")
	colPanel  = lipgloss.Color("#16181d")
	colFg     = lipgloss.Color("#d6d3cd")
	colMuted  = lipgloss.Color("#6f6b64")
	colBorder = lipgloss.Color("#2a2d34")
	colAccent = lipgloss.Color("#c4a574")
	colUser   = lipgloss.Color("#d6d3cd")
	colAsst   = lipgloss.Color("#c8c4bc")
	colTool   = lipgloss.Color("#8a9a7b")
	colErr    = lipgloss.Color("#c4785a")
	colOk     = lipgloss.Color("#7d9a6e")
	colQuit   = lipgloss.Color("#c4453c")

	stApp = lipgloss.NewStyle().Background(colBg).Foreground(colFg)

	stSide      = lipgloss.NewStyle().Background(colPanel).Foreground(colFg)
	stSideTitle = lipgloss.NewStyle().Bold(true).Foreground(colAccent).Background(colPanel)
	stMuted     = lipgloss.NewStyle().Foreground(colMuted)
	stSel       = lipgloss.NewStyle().Bold(true).Foreground(colBg).Background(colAccent)
	stItem      = lipgloss.NewStyle().Foreground(colFg)
	stHover     = lipgloss.NewStyle().Foreground(colAccent).Bold(true)

	stUserBubble = lipgloss.NewStyle().Foreground(colUser).Background(lipgloss.Color("#1c1f26")).Padding(0, 1)
	stAsstBubble = lipgloss.NewStyle().Foreground(colAsst).Padding(0, 1)
	stToolCard   = lipgloss.NewStyle().Foreground(colTool).Border(lipgloss.NormalBorder()).BorderForeground(colBorder).Padding(0, 1)
	stErr        = lipgloss.NewStyle().Foreground(colErr)
	stOk         = lipgloss.NewStyle().Foreground(colOk)
	stSys        = lipgloss.NewStyle().Foreground(colMuted).Italic(true)

	stComposer = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colBorder).Background(colPanel)
	stBtn      = lipgloss.NewStyle().Foreground(colBg).Background(colAccent).Padding(0, 1).Bold(true)
	stBtnGhost = lipgloss.NewStyle().Foreground(colAccent).Border(lipgloss.NormalBorder()).BorderForeground(colBorder).Padding(0, 1)

	stChip = lipgloss.NewStyle().Foreground(colAccent).Background(colPanel)
	stQuit = lipgloss.NewStyle().Foreground(colQuit).Background(colPanel).Bold(true)
	stStop = lipgloss.NewStyle().Foreground(colQuit).Background(colPanel).Bold(true)

	stModal      = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colAccent).Background(colPanel).Padding(0, 1)
	stField      = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colBorder).Padding(0, 1)
	stFieldFocus = lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(colAccent).Padding(0, 1)
)
