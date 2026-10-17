package tui

import (
	"strings"
	"testing"
	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/tobiasGuta/Docktop/internal/dockercli"
)

func imageReviewFixture() Model {
	m:=testModel()
	m.page=2
	m.snapshot.Images=[]dockercli.Image{{ID:"abcdefabcdef",Repository:"hello-world",Tag:"latest",Size:"21.8kB"}}
	return m
}
func TestImageRemovalRequiresReviewAndY(t *testing.T) {
	m:=imageReviewFixture()
	v,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'d'}})
	m=v.(Model)
	if cmd!=nil||!m.imageRemoval.open||m.imageRemoval.phase!="review"||!strings.Contains(m.View(),"hello-world:latest")||!strings.Contains(m.View(),"default") {
		t.Fatal("missing exact image removal review")
	}
	v,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'n'}})
	m=v.(Model)
	if m.imageRemoval.open||cmd!=nil {t.Fatal("cancellation mutated image")}
	v,_=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'d'}})
	m=v.(Model)
	v,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}})
	m=v.(Model)
	if m.imageRemoval.phase!="applying"||cmd==nil {t.Fatal("approval did not initiate one operation")}
	v,cmd=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}})
	if cmd!=nil {t.Fatal("duplicate write while busy")}
}
func TestImageRemovalResultAndStaleResponse(t *testing.T) {
	m:=imageReviewFixture()
	v,_:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'d'}})
	m=v.(Model)
	v,_=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'y'}})
	m=v.(Model)
	target:=m.imageRemoval.target
	v,cmd:=m.Update(imageRemovalDoneMsg{target:target,result:dockercli.ImageRemovalResult{Outcome:"verified",Reason:"Tag removed."}})
	m=v.(Model)
	if cmd==nil||m.imageRemoval.outcome!="verified"||!strings.Contains(m.View(),"Tag removed.") {
		t.Fatal("verified response must be shown and refresh image list")
	}
	v,_=m.Update(tea.KeyMsg{Type:tea.KeyEsc})
	m=v.(Model)
	if m.imageRemoval.open {t.Fatal("result modal did not close")}
	v,_=m.Update(imageRemovalDoneMsg{target:target,result:dockercli.ImageRemovalResult{Outcome:"verified"}})
	if v.(Model).imageRemoval.open {t.Fatal("stale result reopened modal")}
}
func TestImageRemovalRejectsDanglingImageWithoutOpeningPrompt(t *testing.T) {
	m:=imageReviewFixture()
	m.snapshot.Images[0].Repository="<none>"
	m.snapshot.Images[0].Tag="<none>"
	v,cmd:=m.Update(tea.KeyMsg{Type:tea.KeyRunes,Runes:[]rune{'d'}})
	m=v.(Model)
	if m.imageRemoval.open||cmd!=nil {t.Fatal("dangling deletion must not be offered")}
}
func TestImageRemovalReviewFitsTerminal(t *testing.T) {
	for _,dim:=range [][2]int{{172,52},{90,24},{50,16},{30,10}} {
		m:=imageReviewFixture()
		m.width,m.height=dim[0],dim[1]
		m.imageRemoval=imageRemovalOverlay{open:true,phase:"review",target:dockercli.ImageRemovalTarget{
			Context:"default",Repository:"hello-world",Tag:"latest",ID:"abcdefabcdef",
		}}
		assertViewFits(t,m,dim[0],dim[1])
	}
}

func assertViewFits(t *testing.T, m Model, width, height int) {
	t.Helper()
	view := m.View()
	if lipgloss.Height(view)>height {t.Fatalf("height %d exceeds %d",lipgloss.Height(view),height)}
	for _,line:=range strings.Split(view,"\n") {
		if lipgloss.Width(line)>width {t.Fatalf("width %d exceeds %d",lipgloss.Width(line),width)}
	}
}
