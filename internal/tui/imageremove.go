package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

type imageRemovalOverlay struct {
	open    bool
	phase   string // review, applying, result
	target  dockercli.ImageRemovalTarget
	outcome string
	notice  string
}

type imageRemovalDoneMsg struct {
	target dockercli.ImageRemovalTarget
	result dockercli.ImageRemovalResult
}

func removeImageTag(c dockercli.Client, target dockercli.ImageRemovalTarget) tea.Cmd {
	return func() tea.Msg {
		return imageRemovalDoneMsg{target: target, result: c.RemoveImageTag(context.Background(), target)}
	}
}

func (m Model) imageRemovalKey(key string) (tea.Model, tea.Cmd) {
	p := &m.imageRemoval
	switch p.phase {
	case "review":
		switch key {
		case "n", "esc":
			m.imageRemoval = imageRemovalOverlay{}
		case "y", "Y":
			if !m.actionReviewFits(m.imageReviewLines()) {
				return m, nil
			}
			p.phase = "applying"
			return m, removeImageTag(m.client, p.target)
		}
	case "result":
		if key == "enter" || key == "esc" || key == "q" {
			m.imageRemoval = imageRemovalOverlay{}
		}
		// While applying, no key can trigger another Docker write.
	}
	return m, nil
}

func (m Model) imageRemovalResult(v imageRemovalDoneMsg) (Model, tea.Cmd) {
	p := &m.imageRemoval
	if !p.open || p.phase != "applying" || p.target != v.target {
		return m, nil
	}
	p.phase = "result"
	p.outcome = v.result.Outcome
	p.notice = v.result.Reason
	if p.outcome == "verified" {
		// Refresh the ordinary snapshot only after Docker readback verified
		// that the selected repository:tag no longer exists.
		cmd := m.refresh(true)
		return m, cmd
	}
	return m, nil
}

func (m Model) imageRemovalFooter() string {
	switch m.imageRemoval.phase {
	case "review":
		if !m.actionReviewFits(m.imageReviewLines()) {
			return "[Esc] Cancel  •  Enlarge terminal"
		}
		return "[y] Remove this exact image tag  [n/Esc] Cancel"
	case "applying":
		return "[Wait] Checking context, target and removal readback"
	default:
		return "[Esc/Enter] Return to Images"
	}
}

func (m Model) imageReviewLines() []string {
	target := m.imageRemoval.target
	return reviewLines(m.width,
		"Image: "+target.Reference(),
		"Context: "+target.Context,
		"ID: "+target.ID,
		"Removing the last tag also removes the image.",
		"Untagged parent images are preserved.",
	)
}

func (m Model) imageRemovalView(w, h int) string {
	p := m.imageRemoval
	var lines []string
	switch p.phase {
	case "review":
		lines = m.imageReviewLines()
	case "applying":
		lines = []string{amber.Render("Checking target and removal result…")}
	case "result":
		lines = append([]string{"RESULT: " + p.outcome}, reviewLines(w, p.notice)...)
	}
	return frame("IMAGES / REVIEW TAG REMOVAL", lines, w, h)
}
