package main

import (
	"fmt"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

const columnButtonWidth = 6

func (v *syntaxView) columnBarRect() (x, y, width int) {
	x, top, width, height := v.GetRect()
	if width < 20 || height < 3 || v.GetTextLength() == 0 {
		return 0, 0, 0
	}
	return x + 1, top + height - 1, width - 2
}

func (v *syntaxView) drawColumnBar(screen tcell.Screen) {
	x, y, width := v.columnBarRect()
	if width == 0 {
		return
	}
	_, column := v.GetOffset()
	style := tcell.StyleDefault.Foreground(muted).Background(panel)
	for col := x; col < x+width; col++ {
		screen.SetContent(col, y, ' ', nil, style)
	}
	tview.Print(screen, "◀", x, y, columnButtonWidth, tview.AlignCenter, accent)
	tview.Print(screen, fmt.Sprintf("가로 이동 · %d열", column+1), x+columnButtonWidth, y, width-2*columnButtonWidth, tview.AlignCenter, muted)
	tview.Print(screen, "▶", x+width-columnButtonWidth, y, columnButtonWidth, tview.AlignCenter, accent)
}

func (v *syntaxView) scrollColumns(delta int) {
	_, _, width, _ := v.GetInnerRect()
	if width <= 0 {
		return
	}
	if revision := v.revision.Load(); v.columnsRevision != revision || v.columnsTabSize != tview.TabSize {
		// Measure only when scrolling, so typing does not rescan the full document.
		v.columnsRevision, v.columnsTabSize = revision, tview.TabSize
		v.columns = 0
		for text := v.GetText(); text != ""; {
			var line string
			line, text, _ = strings.Cut(text, "\n")
			columns := uniseg.StringWidth(line) + strings.Count(line, "\t")*tview.TabSize
			v.columns = max(v.columns, columns)
		}
	}
	row, column := v.GetOffset()
	v.SetOffset(row, min(max(0, column+delta), max(0, v.columns+1-width)))
}

func (v *syntaxView) pageColumns(direction int) {
	_, _, width, _ := v.GetInnerRect()
	v.scrollColumns(direction * max(4, width/2))
}

func (v *syntaxView) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	editor := v.TextArea.MouseHandler()
	return func(action tview.MouseAction, e *tcell.EventMouse, focus func(tview.Primitive)) (bool, tview.Primitive) {
		col, row := e.Position()
		if v.scrollDrag {
			switch action {
			case tview.MouseMove, tview.MouseLeftUp:
				v.scrollColumns(col - v.scrollX)
				v.scrollX = col
				if action == tview.MouseLeftUp {
					v.scrollDrag = false
					return true, nil
				}
			}
			return true, v
		}
		if v.InRect(col, row) {
			switch action {
			case tview.MouseScrollLeft:
				v.scrollColumns(-4)
				return true, nil
			case tview.MouseScrollRight:
				v.scrollColumns(4)
				return true, nil
			case tview.MouseScrollUp, tview.MouseScrollDown:
				if e.Modifiers()&tcell.ModShift != 0 {
					delta := 4
					if action == tview.MouseScrollUp {
						delta = -delta
					}
					v.scrollColumns(delta)
					return true, nil
				}
			}
		}
		x, y, width := v.columnBarRect()
		if row == y && col >= x && col < x+width {
			if action == tview.MouseLeftDown {
				switch {
				case col < x+columnButtonWidth:
					v.pageColumns(-1)
				case col >= x+width-columnButtonWidth:
					v.pageColumns(1)
				default:
					v.scrollDrag, v.scrollX = true, col
					return true, v
				}
			}
			return true, nil
		}
		return editor(action, e, focus)
	}
}
