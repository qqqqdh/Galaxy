package main

import (
	"io"
	"sync"
	"sync/atomic"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

type terminalFrame struct {
	cells         []uv.Cell
	width, height int
	cursor        uv.Position
	showCursor    bool
}

func (f *terminalFrame) cellAt(col, row int) *uv.Cell {
	if col < 0 || col >= f.width || row < 0 || row >= f.height {
		return nil
	}
	return &f.cells[row*f.width+col]
}

type terminalWorker struct {
	term      *vt.Emulator
	inputPipe io.Closer
	commands  chan func(*terminalWorker)
	stopped   chan struct{}
	done      chan struct{}
	draw      chan struct{}
	visible   atomic.Bool
	scroll    int
	framesMu  sync.RWMutex
	frame     terminalFrame
	back      terminalFrame
}

func newTerminalWorker(width, height int, output io.Writer, pending chan []byte, stopProcess func()) *terminalWorker {
	em := vt.NewEmulator(width, height)
	w := &terminalWorker{
		term: em, inputPipe: em.InputPipe().(io.Closer), commands: make(chan func(*terminalWorker), 128),
		stopped: make(chan struct{}), done: make(chan struct{}), draw: make(chan struct{}, 1),
	}
	inputDone := make(chan struct{})
	go func() {
		defer close(inputDone)
		relayTerminalInput(output, em, pending, stopProcess)
	}()
	go func() {
		defer func() {
			<-inputDone
			_ = em.Close()
			close(w.done)
		}()
		for {
			select {
			case <-w.stopped:
				return
			default:
			}
			select {
			case <-w.stopped:
				return
			case command := <-w.commands:
				command(w)
			}
		}
	}()
	return w
}

func (w *terminalWorker) close() {
	close(w.stopped)
	_ = w.inputPipe.Close()
}

func (w *terminalWorker) enqueue(command func(*terminalWorker)) bool {
	select {
	case w.commands <- command:
		return true
	default:
		return false
	}
}

func (w *terminalWorker) write(batch []byte) bool {
	done := make(chan struct{})
	select {
	case <-w.stopped:
		return false
	case w.commands <- func(w *terminalWorker) {
		_, _ = w.term.Write(batch)
		w.capture()
		close(done)
	}:
	}
	select {
	case <-done:
		return true
	case <-w.done:
		return false
	}
}

func (w *terminalWorker) setVisible(visible bool) {
	if w.visible.Swap(visible) != visible && visible {
		if !w.enqueue((*terminalWorker).capture) {
			w.visible.Store(false)
		}
	}
}

func (w *terminalWorker) history() int {
	if w.term.IsAltScreen() {
		return 0
	}
	return w.term.ScrollbackLen()
}

func (w *terminalWorker) resetScroll() {
	if w.scroll != 0 {
		w.scroll = 0
		w.capture()
	}
}

func (w *terminalWorker) capture() {
	if !w.visible.Load() {
		return
	}
	em := w.term
	back := &w.back
	back.width, back.height = em.Width(), em.Height()
	count := back.width * back.height
	if cap(back.cells) < count {
		back.cells = make([]uv.Cell, count)
	}
	back.cells = back.cells[:count]
	history := w.history()
	w.scroll = min(history, w.scroll)
	for row := 0; row < back.height; row++ {
		offset := row - w.scroll
		for col := 0; col < back.width; col++ {
			var cell *uv.Cell
			if offset < 0 {
				cell = em.ScrollbackCellAt(col, history+offset)
			} else {
				cell = em.CellAt(col, offset)
			}
			index := row*back.width + col
			back.cells[index] = uv.Cell{}
			if cell != nil {
				back.cells[index] = *cell
			}
		}
	}
	back.cursor = em.CursorPosition()
	back.showCursor = w.scroll == 0 && !em.CursorHidden()
	// UI draws the published frame while the worker fills the other frame.
	w.framesMu.Lock()
	w.frame, w.back = w.back, w.frame
	w.framesMu.Unlock()
	select {
	case w.draw <- struct{}{}:
	default:
	}
}
