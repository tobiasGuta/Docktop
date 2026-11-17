package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

func TestSnapshotHealthAndQueuedRefresh(t *testing.T) {
	m := testModel()
	m.busy = true
	m.message = "Operation complete"
	updated, cmd := m.Update(changedMsg{operation: "start", name: "worker"})
	m = updated.(Model)
	if cmd != nil || !m.refreshQueued || !m.busy {
		t.Fatal("write completion launched an overlapping scan")
	}
	now := time.Now()
	updated, cmd = m.Update(snapshotMsg{snapshot: dockercli.Snapshot{Updated: now}, err: errors.New("offline")})
	m = updated.(Model)
	if cmd == nil || !m.busy || m.refreshQueued || !strings.Contains(m.statusLine(100), "STALE") {
		t.Fatal("failed scan did not show stale state and dispatch the queued refresh")
	}
	updated, _ = m.Update(snapshotMsg{snapshot: dockercli.Snapshot{Context: "default", Updated: now.Add(time.Second)}})
	m = updated.(Model)
	if !strings.Contains(m.statusLine(100), "READY") || strings.Contains(m.View(), "Docker unavailable") {
		t.Fatal("successful retry did not clear the scan failure")
	}
	updated, _ = m.Update(snapshotMsg{snapshot: dockercli.Snapshot{Context: "older", Updated: now}})
	if updated.(Model).snapshot.Context != "default" {
		t.Fatal("old snapshot replaced newer Docker state")
	}
}

func TestStaleHubAndDetailsResponsesIgnored(t *testing.T) {
	m := testModel()
	m.hubRequest = 2
	m.hubBusy = true
	m.detailRequest = 3
	updated, _ := m.Update(hubMsg{request: 1, data: []dockercli.HubImage{{Name: "old"}}})
	m = updated.(Model)
	if !m.hubBusy || len(m.hub) != 0 {
		t.Fatal("stale Hub result updated the model")
	}
	updated, _ = m.Update(inspectMsg{request: 2, title: "old details"})
	if updated.(Model).detailTitle != "" {
		t.Fatal("stale result opened details")
	}
}

func TestComposeDrillDownUsesFilteredContainerSelection(t *testing.T) {
	m := testModel()
	m.page = 6
	m.composeContext = m.snapshot.Context
	m.composeLoaded = true
	m.compose = []dockercli.ComposeProject{{Name: "demo"}}
	m.snapshot.Containers = []dockercli.Container{
		{ID: "a", Names: "other"},
		{ID: "b", Names: "demo-api", Image: "my-api", ComposeProject: "demo"},
		{ID: "c", Names: "demo-db", Image: "postgres", ComposeProject: "demo"},
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.page != 1 || m.rowCount() != 2 {
		t.Fatal("Compose did not open the project's containers")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("postgres"), Paste: true})
	updated, _ = updated.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(Model)
	if m.rowCount() != 1 || m.selectedContainerID() != "c" {
		t.Fatal("container filtering selected the wrong resource")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if updated.(Model).page != 6 {
		t.Fatal("Escape did not return to Compose")
	}
}

func TestImageApprovalRequiresVisibleReview(t *testing.T) {
	m := imageReviewFixture()
	m.width, m.height = 80, 12
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if strings.Contains(updated.View(), "[y]") {
		t.Fatal("confirmation offered when the review does not fit")
	}
	updated, cmd := updated.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cmd != nil || updated.(Model).imageRemoval.phase != "review" {
		t.Fatal("approval accepted when the review does not fit")
	}
	updated, _ = updated.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	if !strings.Contains(updated.View(), "[y]") || !strings.Contains(updated.View(), "Context: default") {
		t.Fatal("full review did not become available after resizing")
	}
}
