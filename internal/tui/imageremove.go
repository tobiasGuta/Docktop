package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

type imageRemovalOverlay struct {
	open   bool
	phase  string // review, applying, result
	target dockercli.ImageRemovalTarget
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
		m.busy = true
		return m, poll(m.client)
	}
	return m, nil
}

func (m Model) imageRemovalFooter() string {
	switch m.imageRemoval.phase {
	case "review":
		return "[y] Remove this exact image tag  [n/Esc] Cancel"
	case "applying":
		return "[Wait] Checking context, target and removal readback"
	default:
		return "[Esc/Enter] Return to Images"
	}
}

func (m Model) imageRemovalView(w, h int) string {
	p := m.imageRemoval
	ref := p.target.Reference()
	lines := []string{
		amber.Render("EXACT IMAGE REMOVAL REVIEW"), "",
		"Repository: " + plain(p.target.Repository, max(8, w-15)),
		"Tag:        " + plain(p.target.Tag, max(8, w-15)),
		"Image ID:   " + plain(p.target.ID, max(8, w-15)),
		"Context:    " + plain(p.target.Context, max(8, w-15)),
		"",
		"Command: docker --context " + plain(p.target.Context, max(8, w-15)) + " image rm -- " + plain(ref, max(8, w-15)),
		"",
		"No force, pruning, container changes or volume deletion.",
		"Removing a tag may retain image layers referenced elsewhere.",
		"Context and exact reference/ID are checked before execution.",
	}
	switch p.phase {
	case "review":
		lines = append(lines, "", red.Render("Press y to attempt removal; n/Esc cancels."))
	case "applying":
		lines = append(lines, "", amber.Render("Validating and applying approved operation..."))
	case "result":
		lines = append(lines, "", "RESULT: "+plain(p.outcome, 24),
			plain(p.notice, max(8, w-4)), "",
			"Only VERIFIED means the selected reference disappeared.")
	}
	return frame("IMAGES / REVIEW TAG REMOVAL", lines, w, h)
}
