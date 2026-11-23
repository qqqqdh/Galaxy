package main

import (
	"testing"

	"github.com/charmbracelet/x/vt"
)

func TestClaudeCompositionUsesPromptCaret(t *testing.T) {
	em := vt.NewEmulator(40, 8)
	t.Cleanup(func() { _ = em.Close() })
	_, _ = em.Write([]byte("\x1b[?25l❯ 한글\r\nabc\x1b[7m \x1b[27m\r\n──────\r\n\x1b[7mfooter\x1b[27m"))
	p := claudePromptCaret(em.Width(), em.Height(), em.CellAt)
	if !em.CursorHidden() || p == nil || p.X != 3 || p.Y != 1 || em.CursorPosition() == *p {
		t.Fatalf("prompt caret = %v, terminal cursor = %v", p, em.CursorPosition())
	}
}
