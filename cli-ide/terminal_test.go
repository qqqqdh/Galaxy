package main

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestAssistantOutputRedrawsOnlyItsPane(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	base := tcell.NewSimulationScreen("UTF-8")
	i.inputScreen = &inputScreen{Screen: base}
	i.app.SetScreen(i.inputScreen)
	if i.inputScreen.initErr != nil {
		t.Fatal(i.inputScreen.initErr)
	}
	t.Cleanup(base.Fini)
	base.SetSize(120, 40)
	i.newFile()
	i.showAssistant()
	worker := newTerminalWorker(2, 1, io.Discard, make(chan []byte, 128), func() {})
	i.assistant.worker = worker
	t.Cleanup(i.assistant.close)
	i.app.ForceDraw()

	base.SetContent(0, 0, '#', nil, tcell.StyleDefault)
	if !worker.write([]byte("output")) {
		t.Fatal("terminal worker stopped before publishing output")
	}
	i.redrawTerminal(i.assistant)
	x, y, _, _ := i.assistant.GetInnerRect()
	cells, width, _ := base.GetContents()
	if string(cells[0].Runes) != "#" || string(cells[y*width+x].Runes) != "o" {
		t.Fatal("assistant update should paint its output without redrawing the header")
	}
	i.app.SetFocus(i.active.editor)
	key := tcell.NewEventKey(tcell.KeyRune, '한', tcell.ModNone)
	key = i.app.GetInputCapture()(key)
	i.active.editor.InputHandler()(key, func(tview.Primitive) {})
	i.app.ForceDraw()
	cells, _, _ = base.GetContents()
	if i.active.editor.GetText() != "한" || string(cells[0].Runes) != "#" || string(cells[y*width+x].Runes) != "o" {
		t.Fatal("editor key should update the editor without clearing the existing terminal frame")
	}
}

func TestTerminalOutputGroupsBurstAndFlushesFinalBytes(t *testing.T) {
	output := make(chan []byte, 6)
	var want, got []byte
	for value := byte('a'); value <= 'f'; value++ {
		chunk := bytes.Repeat([]byte{value}, terminalOutputChunk)
		output <- chunk
		want = append(want, chunk...)
	}
	close(output)
	var sizes []int
	relayTerminalOutput(output, func(batch []byte) {
		sizes = append(sizes, len(batch))
		got = append(got, batch...)
	})
	if len(sizes) != 2 || sizes[0] != terminalOutputBatch || sizes[1] != 2*terminalOutputChunk {
		t.Fatalf("unexpected output batch sizes: %v", sizes)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("PTY output was lost or reordered")
	}
}

func TestTerminalInputDoesNotWaitForPTY(t *testing.T) {
	emulator, input := io.Pipe()
	output, pty := io.Pipe()
	defer emulator.Close()
	defer input.Close()
	defer output.Close()
	defer pty.Close()
	overflow := make(chan struct{}, 1)
	go relayTerminalInput(pty, emulator, make(chan []byte, 16), func() { overflow <- struct{}{} })

	want := bytes.Repeat([]byte("paste "), terminalInputChunk)
	written := make(chan error, 1)
	go func() {
		_, err := input.Write(want)
		written <- err
		_ = input.Close()
	}()
	select {
	case err := <-written:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("emulator input waited for the blocked PTY")
	}

	got := make([]byte, len(want))
	read := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(output, got)
		read <- err
	}()
	select {
	case err := <-read:
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("PTY did not receive the paste intact: %v", err)
		}
	case <-overflow:
		t.Fatal("test paste overflowed the terminal input queue")
	case <-time.After(2 * time.Second):
		t.Fatal("PTY did not receive the paste")
	}
}
