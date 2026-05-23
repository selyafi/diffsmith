package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/selyafi/diffsmith/internal/review"
)

// LoaderModel wraps the review Model so the TUI can launch immediately
// when the user runs `diffsmith review` and stream pipeline phases
// (fetch → model → validate) through an animated spinner + status text
// before findings arrive. Once LoadReadyMsg fires, View delegates to the
// inner ReviewModel for the rest of the session.
//
// Pattern: a goroutine in the app layer drives the pipeline and pushes
// messages via tea.Program.Send. The loader stays in the bubbletea event
// loop the whole time, so the spinner keeps animating without any extra
// goroutine inside the TUI.
type LoaderModel struct {
	spinner spinner.Model
	phase   string
	err     error

	review        *Model
	quarantined   []review.Quarantined
	totalReviewed int
}

// PhaseStatusMsg updates the loader's status text.
type PhaseStatusMsg string

// LoadErrorMsg surfaces a pipeline failure. The loader renders the error
// and waits for the user to dismiss with q/ctrl+c.
type LoadErrorMsg struct{ Err error }

// LoadReadyMsg hands validated findings + quarantined ones to the loader.
// After this arrives the loader transitions to the inner ReviewModel.
type LoadReadyMsg struct {
	Findings    []review.Finding
	Quarantined []review.Quarantined
}

// NewLoaderModel constructs a LoaderModel with a default-styled spinner
// and an initial status line. The status updates as PhaseStatusMsg
// messages arrive from the pipeline.
func NewLoaderModel(initialStatus string) *LoaderModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))
	return &LoaderModel{
		spinner: s,
		phase:   initialStatus,
	}
}

// Init starts the spinner tick.
func (m *LoaderModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update routes messages either to the loader (during the wait) or to
// the inner ReviewModel (after findings have arrived). The transition is
// one-way: once a LoadReadyMsg arrives we hand off and never look back.
func (m *LoaderModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Once findings are loaded, the inner review model owns input.
	if m.review != nil {
		newReview, cmd := m.review.Update(msg)
		// Update returns the model as a tea.Model; in our case it's
		// always *Model since we only delegate to it. Re-assign to keep
		// the loader's pointer fresh.
		if rm, ok := newReview.(*Model); ok {
			m.review = rm
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case PhaseStatusMsg:
		m.phase = string(msg)
		return m, nil

	case LoadErrorMsg:
		m.err = msg.Err
		// Leave the spinner running until the user dismisses, so the
		// error message is visible without the program quitting first.
		return m, nil

	case LoadReadyMsg:
		m.quarantined = msg.Quarantined
		m.totalReviewed = len(msg.Findings)
		m.review = NewModel(msg.Findings)
		return m, m.review.Init()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the loading state or delegates to the inner ReviewModel.
func (m *LoaderModel) View() string {
	if m.review != nil {
		return m.review.View()
	}
	if m.err != nil {
		return loaderErrorStyle.Render(fmt.Sprintf("\n  ✗  %v\n\n  Press q or ctrl+c to exit.\n", m.err))
	}
	return loaderStyle.Render(fmt.Sprintf("\n  %s  %s\n\n  Press q or ctrl+c to cancel.\n",
		m.spinner.View(), m.phase))
}

// GetApprovedFindings delegates to the inner ReviewModel; returns nil
// before findings have loaded.
func (m *LoaderModel) GetApprovedFindings() []review.Finding {
	if m.review == nil {
		return nil
	}
	return m.review.GetApprovedFindings()
}

// GetFindingsMarkedForPost delegates to the inner ReviewModel; returns
// nil before findings have loaded.
func (m *LoaderModel) GetFindingsMarkedForPost() []review.Finding {
	if m.review == nil {
		return nil
	}
	return m.review.GetFindingsMarkedForPost()
}

// Quarantined returns the validator-rejected findings (or nil before
// load completes).
func (m *LoaderModel) Quarantined() []review.Quarantined { return m.quarantined }

// TotalReviewed is the number of valid findings handed to the inner
// ReviewModel (i.e., what the user saw in the TUI), used by writeFindings
// to disambiguate "model returned nothing" vs "user approved nothing".
func (m *LoaderModel) TotalReviewed() int { return m.totalReviewed }

// Err returns any pipeline error pushed via LoadErrorMsg. The app layer
// reads this after the TUI quits and propagates as the process exit code.
func (m *LoaderModel) Err() error { return m.err }

// ReviewModel returns the inner ReviewModel once findings have loaded,
// or nil before LoadReadyMsg arrives. Used by the app-layer test seam to
// drive the existing test fakes against the inner model without exposing
// the loader's transition wiring to tests.
func (m *LoaderModel) ReviewModel() *Model { return m.review }

var (
	loaderStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")).
			Padding(1, 2)

	loaderErrorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("9")).
				Padding(1, 2)
)
