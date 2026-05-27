package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/selyafi/diffsmith/internal/review"
)

// Pane width budget. Chosen for a 120-column terminal which is the v1
// target; smaller terminals wrap awkwardly but don't crash. WindowSizeMsg
// handling is deferred to v1.x.
const (
	filesPaneWidth    = 24
	findingsPaneWidth = 36
	detailPaneWidth   = 60
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	paneStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("8")).
			Padding(0, 1)

	activePaneStyle = paneStyle.
			BorderForeground(lipgloss.Color("12")).
			Bold(true)

	rowSelectedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12")).
				Bold(true)

	rowMutedStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	labelStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("14"))

	severityHighStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")).
				Bold(true)
	severityMediumStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("11"))
	severityLowStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8"))
	severitySuggestStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("12"))

	stateApprovedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("10")).
				Bold(true)
	stateDismissedStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("8")).
				Strikethrough(true)

	footerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")).
			Padding(0, 1)

	editModeBadge = lipgloss.NewStyle().
			Foreground(lipgloss.Color("0")).
			Background(lipgloss.Color("11")).
			Padding(0, 1).
			Bold(true)
)

// View renders the three-pane TUI per docs/tui-workflow.md and PRD F8:
// files (left) | findings (middle) | detail or editor (right). The detail
// pane swaps to the textarea when editMode is true (PRD F9).
func (m *Model) View() string {
	if len(m.findings) == 0 {
		// The empty TUI still launches in the loader-driven flow (the
		// pipeline succeeded; the model just returned nothing). Without
		// a quit hint a first-time user has no signal that they need to
		// press q to exit — surface it explicitly.
		return lipgloss.JoinVertical(lipgloss.Left,
			loaderStyle.Render("\n  No findings to review.\n"),
			footerStyle.Render("q quit"),
		)
	}

	title := titleStyle.Render(fmt.Sprintf(
		"Diffsmith Review  (%d findings, %d approved, %d dismissed)",
		len(m.findings), m.ApprovedCount(), m.DismissedCount(),
	))
	if m.editMode {
		title += "  " + editModeBadge.Render("EDIT MODE")
	}

	body := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderFilesPane(),
		m.renderFindingsPane(),
		m.renderDetailPane(),
	)

	return lipgloss.JoinVertical(lipgloss.Left, title, body, m.renderFooter())
}

// renderFilesPane lists the distinct files touched by findings, with the
// per-file finding count. The current finding's file is highlighted so
// the reviewer can orient quickly even when scanning the findings pane.
func (m *Model) renderFilesPane() string {
	counts := make(map[string]int)
	order := make([]string, 0)
	for _, f := range m.findings {
		if _, seen := counts[f.File]; !seen {
			order = append(order, f.File)
		}
		counts[f.File]++
	}

	currentFile := ""
	if cur := m.CurrentFinding(); cur != nil {
		currentFile = cur.File
	}

	var b strings.Builder
	b.WriteString(labelStyle.Render("Files") + "\n\n")
	for _, file := range order {
		line := fmt.Sprintf("%s (%d)", file, counts[file])
		if file == currentFile {
			b.WriteString(rowSelectedStyle.Render("▸ "+line) + "\n")
		} else {
			b.WriteString("  " + line + "\n")
		}
	}

	return paneStyle.Width(filesPaneWidth).Height(20).Render(b.String())
}

// renderFindingsPane lists every finding with file:line + title +
// severity + confidence + state badge. The active pane in normal mode.
func (m *Model) renderFindingsPane() string {
	var b strings.Builder
	b.WriteString(labelStyle.Render("Findings") + "\n\n")

	for i, f := range m.findings {
		marker := "  "
		if i == m.selected {
			marker = "▸ "
		}

		line1 := fmt.Sprintf("%s[%d] %s:%d", marker, i+1, f.File, f.Line)
		sev := severityStyle(f.Severity).Render(string(f.Severity.String()))
		line2 := fmt.Sprintf("    %s  (%s, %.0f%%)", truncate(f.Title, 28), sev, f.Confidence*100)
		stateTag := stateBadge(f)
		if stateTag != "" {
			line2 += "  " + stateTag
		}
		if m.IsPostBoundRow(i) {
			line2 += "  " + labelStyle.Render("[post]")
		}

		if i == m.selected {
			b.WriteString(rowSelectedStyle.Render(line1) + "\n")
		} else {
			b.WriteString(line1 + "\n")
		}
		b.WriteString(line2 + "\n\n")
	}

	style := paneStyle
	if !m.editMode {
		style = activePaneStyle
	}
	return style.Width(findingsPaneWidth).Height(20).Render(b.String())
}

// renderDetailPane shows the full finding context (title, evidence,
// suggested comment, fix hint, severity, confidence, file:line). When in
// edit mode the suggested_comment field becomes a focused textarea.
func (m *Model) renderDetailPane() string {
	cur := m.CurrentFinding()
	if cur == nil {
		return paneStyle.Width(detailPaneWidth).Height(20).Render("(no finding selected)")
	}

	var b strings.Builder
	b.WriteString(labelStyle.Render("Detail") + "\n\n")
	fmt.Fprintf(&b, "%s  %s:%d\n", labelStyle.Render("Location:"), cur.File, cur.Line)
	fmt.Fprintf(&b, "%s  %s  (%.0f%% confidence)\n",
		labelStyle.Render("Severity:"),
		severityStyle(cur.Severity).Render(cur.Severity.String()),
		cur.Confidence*100,
	)
	if cur.Model != "" {
		fmt.Fprintf(&b, "%s     %s\n", labelStyle.Render("Model:"), cur.Model)
	}
	state := stateBadge(*cur)
	if state != "" {
		fmt.Fprintf(&b, "%s     %s\n", labelStyle.Render("State:"), state)
	}
	b.WriteString("\n")

	fmt.Fprintf(&b, "%s\n%s\n\n", labelStyle.Render("Title"), cur.Title)
	fmt.Fprintf(&b, "%s\n%s\n\n", labelStyle.Render("Evidence"), wrap(cur.Evidence, 54))

	if m.editMode {
		b.WriteString(labelStyle.Render("Suggested Comment  (edit; esc=cancel, ctrl+s=save)") + "\n")
		b.WriteString(m.editor.View() + "\n\n")
	} else {
		b.WriteString(labelStyle.Render("Suggested Comment") + "\n")
		b.WriteString(wrap(cur.SuggestedComment, 54) + "\n\n")
	}

	if cur.FixHint != "" {
		fmt.Fprintf(&b, "%s\n%s\n", labelStyle.Render("Fix Hint  (read-only)"), wrap(cur.FixHint, 54))
	}

	style := paneStyle
	if m.editMode {
		style = activePaneStyle
	}
	return style.Width(detailPaneWidth).Height(20).Render(b.String())
}

func (m *Model) renderFooter() string {
	if m.editMode {
		return footerStyle.Render("Edit:  ctrl+s = save  |  esc = cancel  |  arrows = move cursor")
	}
	// A transient status preempts the keybinding hint for one tick so
	// the user sees feedback from their last action (copy success or
	// failure, etc.). It clears at the top of the next Update.
	if m.transientStatus != "" {
		return footerStyle.Render(m.transientStatus)
	}
	return footerStyle.Render(
		"↑↓ navigate  |  e edit  |  a approve  |  d dismiss  |  c copy  |  p post (after a)  |  q back",
	)
}

func severityStyle(s review.Severity) lipgloss.Style {
	switch s {
	case review.SeverityHigh:
		return severityHighStyle
	case review.SeverityMedium:
		return severityMediumStyle
	case review.SeverityLow:
		return severityLowStyle
	case review.SeveritySuggestion:
		return severitySuggestStyle
	default:
		return rowMutedStyle
	}
}

func stateBadge(f review.Finding) string {
	switch f.State {
	case review.StateApproved:
		return stateApprovedStyle.Render("✓ approved")
	case review.StateDismissed:
		return stateDismissedStyle.Render("✗ dismissed")
	default:
		return ""
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return s[:n-1] + "…"
}

// wrap is a small word-aware wrapper used for the detail pane body. Lip
// Gloss handles boundary wrapping at the pane width too, but pre-wrapping
// here keeps lines from breaking mid-word.
func wrap(s string, width int) string {
	if width <= 0 || len(s) <= width {
		return s
	}
	var b strings.Builder
	var col int
	for _, word := range strings.Fields(s) {
		if col == 0 {
			b.WriteString(word)
			col = len(word)
			continue
		}
		if col+1+len(word) > width {
			b.WriteString("\n")
			b.WriteString(word)
			col = len(word)
			continue
		}
		b.WriteString(" ")
		b.WriteString(word)
		col += 1 + len(word)
	}
	return b.String()
}
