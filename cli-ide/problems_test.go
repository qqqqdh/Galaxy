package main

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"path/filepath"
	"testing"
)

func TestDiagnosticUnderlineAndJump(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "한글.c")
	i := newIDE(root, config{IndentSize: 4})
	i.addDocument(&document{path: path, text: "//\n\t/*한*/ bad\n"})
	items := parseProblems([]byte(`[{"kind":"error","message":"bad identifier","locations":[{"caret":{"file":"<stdin>","line":2,"byte-column":10},"finish":{"line":2,"byte-column":12}}]}]`), root, path)
	i.showProblems(items, "")
	if len(items) != 1 || len(i.active.syntax.marks) != 1 {
		t.Fatal(items)
	}
	m := i.active.syntax.marks[0]
	if m.row != 1 || m.start != 11 || m.end != 14 {
		t.Fatalf("unexpected screen mark %+v", m)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	i.active.syntax.SetRect(0, 0, 40, 6)
	i.active.syntax.Draw(screen)
	x, y, _, _ := i.active.syntax.GetInnerRect()
	_, _, style, _ := screen.GetContent(x+11, y+1)
	fg, _, attrs := style.Decompose()
	if fg != m.color || attrs&tcell.AttrUnderline == 0 {
		t.Fatal("missing red underline")
	}
	i.openDock("문제")
	i.problemList.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { i.app.SetFocus(p) })
	_, pos, _ := i.active.editor.GetSelection()
	if pos != 12 {
		t.Fatalf("cursor byte offset %d", pos)
	}
	i.showProblems(nil, "문제 없음")
	if len(i.active.syntax.marks) != 0 {
		t.Fatal("stale mark")
	}
}
