package main

import (
	"bytes"
	"fmt"
	"image/color"
	"io"
	"os"
	"os/exec"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/creack/pty"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	terminalInputChunk    = 32 * 1024
	terminalOutputChunk   = 8 * 1024
	terminalOutputBatch   = 32 * 1024
	terminalFrameInterval = 24 * time.Millisecond
)

type shellPane struct {
	*tview.Box
	app           *tview.Application
	worker        *terminalWorker
	file          *os.File
	process       *os.Process
	input         chan []byte
	notify        func(string)
	visible       func() bool
	redraw        func(*shellPane)
	screen        func() *inputScreen
	message       string
	width, height int
	claude        bool
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
	s.file, s.message = file, ""
	s.width, s.height = w, h
	s.claude = args[0] == "claude"
	s.process = cmd.Process
	s.input = make(chan []byte, 128)
	worker := newTerminalWorker(w, h, file, s.input, func() {
		_ = cmd.Process.Kill()
		_ = file.Close()
	})
	s.worker = worker
	worker.setVisible(s.visible())
	go func() {
		for {
			select {
			case <-worker.stopped:
				return
			case <-worker.draw:
				s.app.QueueUpdate(func() {
					if s.worker == worker {
						s.redraw(s)
					}
				})
			}
		}
	}()
	output := make(chan []byte, 8)
	go func() {
		defer close(output)
		buf := make([]byte, terminalOutputChunk)
		for {
			n, err := file.Read(buf)
			if n > 0 {
				output <- bytes.Clone(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		relayTerminalOutput(output, func(batch []byte) {
			worker.write(batch)
		})
		err := cmd.Wait()
		s.app.QueueUpdateDraw(func() {
			if s.file == file {
				if onExit != nil {
					go file.Close()
					_ = worker.inputPipe.Close()
					s.file, s.process, s.input = nil, nil, nil
					onExit(err)
				} else {
					s.process = nil // Wait already reaped the process.
					s.close()
					s.message = "셸 종료 · 터미널 탭을 누르면 다시 엽니다"
				}
			}
		})
	}()
}

// Draw the first output promptly, then group reads that arrive during the next frame.
func relayTerminalOutput(output <-chan []byte, publish func([]byte)) {
	var nextFrame time.Time
	batch := make([]byte, 0, terminalOutputBatch)
	for first := range output {
		if wait := time.Until(nextFrame); wait > 0 {
			time.Sleep(wait)
		}
		batch = append(batch[:0], first...)
	collect:
		for len(batch)+terminalOutputChunk <= terminalOutputBatch {
			select {
			case more, ok := <-output:
				if !ok {
					break collect
				}
				batch = append(batch, more...)
			default:
				break collect
			}
		}
		nextFrame = time.Now().Add(terminalFrameInterval)
		publish(batch)
	}
}

func (s *shellPane) close() {
	if file, process := s.file, s.process; file != nil {
		s.file, s.process, s.input = nil, nil, nil
		if process != nil {
			_ = process.Kill()
		}
		go file.Close()
	}
	if s.worker != nil {
		s.worker.close()
		s.worker = nil
	}
	s.claude = false
}

// Drain the emulator's synchronous input pipe even when a child stops reading the PTY.
func relayTerminalInput(file io.Writer, term io.Reader, pending chan []byte, stop func()) {
	go func() {
		for input := range pending {
			if _, err := file.Write(input); err != nil {
				return
			}
		}
	}()
	defer close(pending)
	buffer := make([]byte, terminalInputChunk)
	full := false
	for {
		n, err := term.Read(buffer)
		if n > 0 && !full {
			select {
			case pending <- bytes.Clone(buffer[:n]):
			default:
				full = true
				go stop()
			}
		}
		if err != nil {
			return
		}
	}
}

func termColor(c color.Color, fallback tcell.Color) tcell.Color {
	if c == nil {
		return fallback
	}
	r, g, b, _ := c.RGBA()
	return tcell.NewRGBColor(int32(r>>8), int32(g>>8), int32(b>>8))
}

func (s *shellPane) Draw(screen tcell.Screen) {
	s.Box.DrawForSubclass(screen, s)
	x, y, w, h := s.GetInnerRect()
	if w < 2 || h < 1 {
		return
	}
	worker := s.worker
	if worker == nil {
		tview.Print(screen, tview.Escape(s.message), x, y, w, tview.AlignLeft, muted)
		return
	}
	if w != s.width || h != s.height {
		file, width, height := s.file, w, h
		if worker.enqueue(func(worker *terminalWorker) {
			worker.term.Resize(width, height)
			if file != nil {
				_ = pty.Setsize(file, &pty.Winsize{Cols: uint16(width), Rows: uint16(height)})
			}
			worker.capture()
		}) {
			s.width, s.height = w, h
		}
	}
	worker.framesMu.RLock()
	defer worker.framesMu.RUnlock()
	frame := &worker.frame
	w, h = min(w, frame.width), min(h, frame.height)
	var link terminalLink
	if s.claude {
		link = findClaudeAuthorization(w, h, frame.cellAt)
	}
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			cell := frame.cellAt(col, row)
			if cell == nil || cell.Width == 0 || cell.Width > w-col {
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
	if s.file != nil && s.HasFocus() && frame.showCursor && frame.cursor.X < w && frame.cursor.Y < h {
		p := frame.cursor
		screen.ShowCursor(x+p.X, y+p.Y)
	}
}

func (i *ide) redrawTerminal(pane *shellPane) {
	if !pane.visible() {
		return
	}
	screen := i.inputScreen
	if screen == nil || i.dialog || i.acceptHint != nil {
		i.app.ForceDraw()
		return
	}
	_, _, w, h := i.pages.GetRect()
	if width, height := screen.Size(); w != width || h != height {
		i.app.ForceDraw()
		return
	}
	if pane.HasFocus() {
		screen.HideCursor()
	}
	pane.Draw(screen)
	screen.Show()
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
		if !s.send(func(worker *terminalWorker) {
			worker.resetScroll()
			worker.term.SendKey(key)
		}) {
			return
		}
		if screen := s.screen(); screen != nil {
			// The worker paints after leaving scrollback or receiving the prompt echo.
			screen.skipFrame = true
		}
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
		if s.worker == nil || col < x || row < y || col >= x+w || row >= y+h {
			return action == tview.MouseLeftDown, nil
		}
		mouse := uv.Mouse{X: col - x, Y: row - y, Button: uv.MouseLeft}
		switch action {
		case tview.MouseLeftDown, tview.MouseLeftUp:
			if s.file != nil {
				s.send(func(worker *terminalWorker) {
					if worker.scroll == 0 {
						if action == tview.MouseLeftDown {
							worker.term.SendMouse(uv.MouseClickEvent(mouse))
						} else {
							worker.term.SendMouse(uv.MouseReleaseEvent(mouse))
						}
					}
				})
			}
		case tview.MouseScrollUp, tview.MouseScrollDown:
			mouse.Button = uv.MouseWheelUp
			delta := 3
			if action == tview.MouseScrollDown {
				mouse.Button, delta = uv.MouseWheelDown, -3
			}
			running := s.file != nil
			s.send(func(worker *terminalWorker) {
				if history := worker.history(); history > 0 {
					worker.scroll = max(0, min(history, worker.scroll+delta))
					worker.capture()
				} else if running {
					worker.term.SendMouse(uv.MouseWheelEvent(mouse))
				}
			})
		default:
			return false, nil
		}
		return true, nil
	})
}

func (s *shellPane) PasteHandler() func(string, func(tview.Primitive)) {
	return s.WrapPasteHandler(func(text string, _ func(tview.Primitive)) {
		if s.file == nil {
			return
		}
		// Leave room for the bracketed-paste markers and a preceding input chunk.
		if len(text)/terminalInputChunk+4 > cap(s.input)-len(s.input) {
			s.notify("터미널 입력이 많습니다 · 붙여넣을 내용을 나눠 주세요")
			return
		}
		s.send(func(worker *terminalWorker) {
			worker.resetScroll()
			worker.term.Paste(text)
		})
	})
}

func (s *shellPane) send(command func(*terminalWorker)) bool {
	if s.worker.enqueue(command) {
		return true
	}
	s.notify("터미널 입력 대기열이 가득 찼습니다")
	return false
}

func (i *ide) makeDock() *tview.Flex {
	i.problemList = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	i.shell = &shellPane{Box: tview.NewBox(), app: i.app, notify: i.notify, redraw: i.redrawTerminal, screen: func() *inputScreen { return i.inputScreen }, visible: func() bool {
		page, _ := i.dock.GetFrontPage()
		return i.fullTerminal || i.dockOpen && page == "터미널"
	}}
	i.dock = tview.NewPages().AddPage("문제", i.problemList, true, true).AddPage("터미널", i.shell, true, false)
	i.dock.AddPage("검색", i.makeSearchPane(), true, false)
	i.changeList = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	i.dock.AddPage("변경사항", i.changeList, true, false)
	i.testList = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	i.testList.AddItem("실행한 테스트가 없습니다", "", 0, nil)
	i.dock.AddPage("테스트 결과", i.testList, true, false)
	i.hints = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	i.hints.SetBorder(true).SetTitle(" 자동완성 ").SetBackgroundColor(panel)
	i.problemButton = tview.NewButton("문제 (0)").SetSelectedFunc(func() { i.openDock("문제") })
	bar := tview.NewFlex().AddItem(i.problemButton, 0, 14, false).
		AddItem(tview.NewButton("터미널").SetSelectedFunc(func() { i.openDock("터미널") }), 0, 12, false).
		AddItem(tview.NewButton("터미널 확대").SetSelectedFunc(i.toggleFullTerminal), 0, 14, false).
		AddItem(tview.NewButton("검색").SetSelectedFunc(func() { i.openDock("검색") }), 0, 9, false).
		AddItem(tview.NewButton("변경사항").SetSelectedFunc(func() { i.openDock("변경사항") }), 0, 12, false).
		AddItem(tview.NewButton("테스트").SetSelectedFunc(func() { i.openDock("테스트 결과") }), 0, 8, false).
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
	} else if page == "테스트 결과" {
		i.app.SetFocus(i.testList)
	} else {
		i.app.SetFocus(i.problemList)
	}
}

func (i *ide) hideDock() {
	i.clearHints()
	i.dockOpen = false
	i.app.SetFocus(i.active.editor)
}
