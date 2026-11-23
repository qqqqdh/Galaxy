package main

import (
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type shellPane struct {
	*tview.Box
	app     *tview.Application
	term    *vt.Emulator
	file    *os.File
	message string
	scroll  int
	claude  bool
	draftAt *uv.Position
}

func (s *shellPane) start(dir string) {
	s.run(dir, shellTask(), nil)
}

func (s *shellPane) run(dir string, job task, onExit func(error)) {
	if s.file != nil {
		return
	}
	s.close()
	args := job.Args
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	_, _, w, h := s.GetInnerRect()
	w, h = max(w, 2), max(h, 1)
	file, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
	if err != nil {
		s.message = fmt.Sprintf("%s 시작 실패 · %v", job.Name, err)
		return
	}
	em := vt.NewEmulator(w, h)
	s.term, s.file, s.message = em, file, ""
	s.claude = args[0] == "claude"
	go func() { _, _ = io.Copy(file, em) }()
	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := file.Read(buf)
			if n > 0 {
				s.app.QueueUpdateDraw(func() {
					if s.file == file {
						_, _ = em.Write(buf[:n])
					}
				})
			}
			if err != nil {
				break
			}
		}
		err := cmd.Wait()
		s.app.QueueUpdateDraw(func() {
			if s.file == file {
				if onExit != nil {
					_ = s.file.Close()
					_ = em.Close()
					s.file = nil
					onExit(err)
				} else {
					s.close()
					s.message = "셸 종료 · 터미널 탭을 누르면 다시 엽니다"
				}
			}
		})
	}()
}

func (s *shellPane) close() {
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	if s.term != nil {
		_ = s.term.Close()
		s.term = nil
	}
	s.scroll = 0
	s.claude = false
}

func termColor(c color.Color, fallback tcell.Color) tcell.Color {
	if c == nil {
		return fallback
	}
	r, g, b, _ := c.RGBA()
	return tcell.NewRGBColor(int32(r>>8), int32(g>>8), int32(b>>8))
}

func (s *shellPane) Draw(screen tcell.Screen) {
	s.draftAt = nil
	s.Box.DrawForSubclass(screen, s)
	x, y, w, h := s.GetInnerRect()
	if w < 2 || h < 1 {
		return
	}
	em := s.term
	if em == nil {
		tview.Print(screen, tview.Escape(s.message), x, y, w, tview.AlignLeft, muted)
		return
	}
	if w != em.Width() || h != em.Height() {
		em.Resize(w, h)
		if s.file != nil {
			_ = pty.Setsize(s.file, &pty.Winsize{Cols: uint16(w), Rows: uint16(h)})
		}
	}
	history := em.ScrollbackLen()
	if em.IsAltScreen() {
		history = 0
	}
	s.scroll = min(history, s.scroll)
	cellAt := func(col, row int) *uv.Cell {
		offset := row - s.scroll
		if offset < 0 {
			return em.ScrollbackCellAt(col, history+offset)
		}
		return em.CellAt(col, offset)
	}
	var link terminalLink
	if s.claude {
		link = findClaudeAuthorization(w, h, cellAt)
	}
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			cell := cellAt(col, row)
			if cell == nil || cell.Width == 0 {
				continue
			}
			r := []rune(cell.Content)
			if len(r) == 0 {
				r = []rune{' '}
			}
			a := cell.Style
			style := tcell.StyleDefault.Foreground(termColor(a.Fg, foreground)).Background(termColor(a.Bg, background)).Bold(a.Attrs&uv.AttrBold != 0).Reverse(a.Attrs&uv.AttrReverse != 0).Underline(a.Underline != 0)
			if link.contains(col, row) {
				style = style.Url(link.url).UrlId("claude-authorization")
			}
			screen.SetContent(x+col, y+row, r[0], r[1:], style)
		}
	}
	if s.file != nil && s.scroll == 0 && s.HasFocus() {
		if !em.CursorHidden() {
			p := em.CursorPosition()
			screen.ShowCursor(x+p.X, y+p.Y)
		} else if s.claude {
			if p := claudePromptCaret(w, h, cellAt); p != nil {
				s.draftAt = &uv.Position{X: x + p.X, Y: y + p.Y}
			}
		}
	}
}

func claudePromptCaret(width, height int, cellAt func(int, int) *uv.Cell) *uv.Position {
	for row := height - 1; row >= 0; row-- {
		marker := cellAt(0, row)
		if marker == nil || marker.Content != "❯" {
			continue
		}
		// Claude hides the hardware cursor and paints one inverse cell in its prompt.
		var caret *uv.Position
		for y := row; y < height; y++ {
			if first := cellAt(0, y); y > row && first != nil && first.Content == "─" {
				break
			}
			for x := 0; x < width; x++ {
				cell := cellAt(x, y)
				if cell == nil || cell.Width == 0 || cell.Style.Attrs&uv.AttrReverse == 0 {
					continue
				}
				if caret != nil {
					return nil
				}
				caret = &uv.Position{X: x, Y: y}
			}
		}
		return caret
	}
	return nil
}

var terminalKeys = map[tcell.Key]rune{
	tcell.KeyEnter: uv.KeyEnter, tcell.KeyTab: uv.KeyTab, tcell.KeyEscape: uv.KeyEscape, tcell.KeyBackspace: uv.KeyBackspace, tcell.KeyBackspace2: uv.KeyBackspace,
	tcell.KeyLeft: uv.KeyLeft, tcell.KeyRight: uv.KeyRight, tcell.KeyUp: uv.KeyUp, tcell.KeyDown: uv.KeyDown, tcell.KeyDelete: uv.KeyDelete, tcell.KeyInsert: uv.KeyInsert,
	tcell.KeyHome: uv.KeyHome, tcell.KeyEnd: uv.KeyEnd, tcell.KeyPgUp: uv.KeyPgUp, tcell.KeyPgDn: uv.KeyPgDown,
}

func (s *shellPane) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return s.WrapInputHandler(func(e *tcell.EventKey, _ func(tview.Primitive)) {
		if s.file == nil {
			return
		}
		key := vt.KeyPressEvent{Code: terminalKeys[e.Key()]}
		switch {
		case e.Key() == tcell.KeyRune:
			key.Code = e.Rune()
			key.Text = string(e.Rune())
		case key.Code != 0:
		case e.Key() >= tcell.KeyCtrlA && e.Key() <= tcell.KeyCtrlZ:
			key.Code = 'a' + rune(e.Key()-tcell.KeyCtrlA)
			key.Mod = uv.ModCtrl
		case e.Key() >= tcell.KeyF1 && e.Key() <= tcell.KeyF12:
			key.Code = uv.KeyF1 + rune(e.Key()-tcell.KeyF1)
		default:
			return
		}
		if e.Modifiers()&tcell.ModAlt != 0 {
			key.Mod |= uv.ModAlt
		}
		s.scroll = 0
		s.term.SendKey(key)
	})
}
func (s *shellPane) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	return s.WrapMouseHandler(func(action tview.MouseAction, e *tcell.EventMouse, focus func(tview.Primitive)) (bool, tview.Primitive) {
		if !s.InRect(e.Position()) {
			return false, nil
		}
		if action == tview.MouseLeftDown {
			focus(s)
		}
		x, y, w, h := s.GetInnerRect()
		col, row := e.Position()
		if s.term == nil || col < x || row < y || col >= x+w || row >= y+h {
			return action == tview.MouseLeftDown, nil
		}
		mouse := uv.Mouse{X: col - x, Y: row - y, Button: uv.MouseLeft}
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftUp:
			if s.file != nil && s.scroll == 0 {
				if action == tview.MouseLeftDown {
					s.term.SendMouse(uv.MouseClickEvent(mouse))
				} else {
					s.term.SendMouse(uv.MouseReleaseEvent(mouse))
				}
			}
		case tview.MouseScrollUp, tview.MouseScrollDown:
			mouse.Button = uv.MouseWheelUp
			delta := 3
			if action == tview.MouseScrollDown {
				mouse.Button, delta = uv.MouseWheelDown, -3
			}
			if history := s.term.ScrollbackLen(); !s.term.IsAltScreen() && history > 0 {
				s.scroll = max(0, min(history, s.scroll+delta))
			} else if s.file != nil {
				s.term.SendMouse(uv.MouseWheelEvent(mouse))
			}
		default:
			return false, nil
		}
		return true, nil
	})
}

func (s *shellPane) PasteHandler() func(string, func(tview.Primitive)) {
	return s.WrapPasteHandler(func(text string, _ func(tview.Primitive)) {
		if s.file != nil {
			s.scroll = 0
			s.term.Paste(text)
		}
	})
}

func (i *ide) makeDock() *tview.Flex {
	i.problemList = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	i.shell = &shellPane{Box: tview.NewBox(), app: i.app}
	i.dock = tview.NewPages().AddPage("문제", i.problemList, true, true).AddPage("터미널", i.shell, true, false)
	i.dock.AddPage("검색", i.makeSearchPane(), true, false)
	i.changeList = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	i.dock.AddPage("변경사항", i.changeList, true, false)
	i.hints = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	i.hints.SetBorder(true).SetTitle(" 자동완성 ").SetBackgroundColor(panel)
	i.problemButton = tview.NewButton("문제 (0)").SetSelectedFunc(func() { i.openDock("문제") })
	bar := tview.NewFlex().AddItem(i.problemButton, 0, 14, false).
		AddItem(tview.NewButton("터미널").SetSelectedFunc(func() { i.openDock("터미널") }), 0, 12, false).
		AddItem(tview.NewButton("터미널 확대").SetSelectedFunc(i.toggleFullTerminal), 0, 14, false).
		AddItem(tview.NewButton("검색").SetSelectedFunc(func() { i.openDock("검색") }), 0, 9, false).
		AddItem(tview.NewButton("변경사항").SetSelectedFunc(func() { i.openDock("변경사항") }), 0, 12, false).
		AddItem(tview.NewButton("접기").SetSelectedFunc(i.hideDock), 0, 9, false)
	i.dockOpen = true
	return bar
}

func (i *ide) toggleFullTerminal() {
	i.clearHints()
	i.fullTerminal = !i.fullTerminal
	if i.fullTerminal {
		i.pages.SwitchToPage("terminal-full")
		i.shell.start(i.root)
		i.app.SetFocus(i.shell)
		return
	}
	i.pages.SwitchToPage("main")
	i.app.SetFocus(i.active.editor)
}

func (i *ide) openDock(page string) {
	i.clearHints()
	i.dockOpen = true
	i.dock.SwitchToPage(page)
	if page == "터미널" {
		i.shell.start(i.root)
		i.app.SetFocus(i.shell)
	} else if page == "검색" {
		i.app.SetFocus(i.searchField)
	} else if page == "변경사항" {
		i.app.SetFocus(i.changeList)
		i.refreshChanges()
	} else {
		i.app.SetFocus(i.problemList)
	}
}

func (i *ide) hideDock() {
	i.clearHints()
	i.dockOpen = false
	i.app.SetFocus(i.active.editor)
}
