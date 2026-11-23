package main

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestInputScreenKeepsFrameUntilClaudeRedraws(t *testing.T) {
	base := tcell.NewSimulationScreen("UTF-8")
	screen := &inputScreen{Screen: base}
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(screen.Fini)
	screen.SetContent(0, 0, 'ㅎ', nil, tcell.StyleDefault)
	screen.Show()

	screen.skipFrame = true
	screen.Clear()
	screen.Show()
	if cells, _, _ := base.GetContents(); screen.skipFrame || len(cells[0].Runes) == 0 || cells[0].Runes[0] != 'ㅎ' {
		t.Fatal("typing frame erased the last visible prompt")
	}

	screen.Clear()
	screen.SetContent(0, 0, '한', nil, tcell.StyleDefault)
	screen.Show()
	if cells, _, _ := base.GetContents(); len(cells[0].Runes) == 0 || cells[0].Runes[0] != '한' {
		t.Fatal("Claude's next prompt update was skipped")
	}
}
