package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/selyafi/diffsmith/internal/provider"
)

// InboxAction encodes how the user exited the inbox screen. The
// top-level loop in app/inbox.go switches on this to decide whether to
// open a review, refresh the list, or quit the program.
type InboxAction int

const (
	InboxActionNone InboxAction = iota
	InboxActionOpen
	InboxActionRefresh
	InboxActionQuit
)

// InboxModel renders a single-pane selectable list of PRSummary rows.
// It is intentionally separate from the review Model so we can run a
// fresh tea.NewProgram for each (per the step-out-between-picks
// pattern from the design spec §7).
type InboxModel struct {
	summaries []provider.PRSummary
	owner     string
	name      string
	selected  int
	action    InboxAction
	pick      *provider.PRSummary
}

func NewInboxModel(summaries []provider.PRSummary, owner, name string) *InboxModel {
	return &InboxModel{
		summaries: summaries,
		owner:     owner,
		name:      name,
	}
}

func (m *InboxModel) Init() tea.Cmd { return nil }

func (m *InboxModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.summaries)-1 {
				m.selected++
			}
		case "enter":
			if len(m.summaries) > 0 {
				p := m.summaries[m.selected]
				m.pick = &p
				m.action = InboxActionOpen
				return m, tea.Quit
			}
		case "r":
			m.action = InboxActionRefresh
			return m, tea.Quit
		case "q", "ctrl+c":
			m.action = InboxActionQuit
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m *InboxModel) View() string {
	if len(m.summaries) == 0 {
		return lipgloss.JoinVertical(lipgloss.Left,
			loaderStyle.Render(fmt.Sprintf("\n  No open PRs in %s/%s.\n", m.owner, m.name)),
			footerStyle.Render("q quit"),
		)
	}

	header := titleStyle.Render(fmt.Sprintf("Inbox: %s/%s  (%d open)", m.owner, m.name, len(m.summaries)))

	var b strings.Builder
	for i, s := range m.summaries {
		marker := "  "
		if i == m.selected {
			marker = "▸ "
		}
		displayTitle := truncate(s.Title, 40)
		draft := ""
		if s.Draft {
			draft = " [d]"
		}
		line := fmt.Sprintf("%s#%d  %s  %s  %s%s",
			marker, s.Number, displayTitle, s.Author, formatAge(s.UpdatedAt), draft)
		if i == m.selected {
			b.WriteString(rowSelectedStyle.Render(line) + "\n")
		} else {
			b.WriteString(line + "\n")
		}
	}

	footer := footerStyle.Render("↑↓ navigate  |  enter open  |  r refresh  |  q quit")
	return lipgloss.JoinVertical(lipgloss.Left, header, b.String(), footer)
}

// Selected returns the current cursor index. Exposed for tests.
func (m *InboxModel) Selected() int { return m.selected }

// Action returns how the user exited the inbox. None until Update sees
// a terminal key.
func (m *InboxModel) Action() InboxAction { return m.action }

// Pick returns the chosen summary, only meaningful when Action() == InboxActionOpen.
func (m *InboxModel) Pick() *provider.PRSummary { return m.pick }

// formatAge renders a duration as "Nm" / "Nh" / "Nd" / "Nw" relative
// to now. Returns "?" if t is zero.
func formatAge(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return fmt.Sprintf("%dw", int(d.Hours()/(24*7)))
	}
}

// ResetSession clears the exit state (action, pick) so a reused model
// can host a fresh Bubble Tea session while keeping the cached list.
// The app reuses one InboxModel across sessions (spec §7: the list
// persists between picks); without this reset a teardown-without-
// keypress after the first open replayed the stale InboxActionOpen and
// re-launched the previous review. diffsmith-qe5.
func (m *InboxModel) ResetSession() {
	m.action = InboxActionNone
	m.pick = nil
}
