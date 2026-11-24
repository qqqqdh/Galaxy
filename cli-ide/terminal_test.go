package main

import (
	"bytes"
	"io"
	"testing"
	"time"
)

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
