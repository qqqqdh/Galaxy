package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/vt"
)

func TestClaudeAuthorizationUsesEntireWrappedLink(t *testing.T) {
	const width, height = 59, 12
	address := "https://claude.com/cai/oauth/authorize?code=true&client_id=test&response_type=code" +
		"&redirect_uri=https%3A%2F%2Fplatform.claude.com%2Foauth%2Fcode%2Fcallback" +
		"&code_challenge=" + strings.Repeat("x", 43) + "&state=" + strings.Repeat("y", 43)
	em := vt.NewEmulator(width, height)
	t.Cleanup(func() { _ = em.Close() })
	_, _ = em.Write([]byte("Claude login\r\n"))
	for remaining := address; len(remaining) > 0; {
		count := min(width, len(remaining))
		_, _ = em.Write([]byte(remaining[:count] + "\r\n"))
		remaining = remaining[count:]
	}
	link := findClaudeAuthorization(width, height, em.CellAt)
	if link.url != address || !link.contains(1, 1) ||
		!link.contains(2, 2) || link.contains(0, 0) || link.contains(link.lastColumn, link.lastRow) {
		t.Fatalf("unexpected link %+v", link)
	}
	if link := findClaudeAuthorization(width, 2, em.CellAt); link.url != "" {
		t.Fatalf("partially visible URI became a link: %s", link.url)
	}
	if completeClaudeAuthorization("https://claude.ai/oauth/authorize?code=true&client_id=9") {
		t.Fatal("incomplete authorization address was accepted")
	}
}

func TestClaudeAuthorizationStopsBeforeInstructions(t *testing.T) {
	const width, height = 180, 5
	for _, padding := range []int{0, 1} {
		prefix := "https://claude.com/cai/oauth/authorize?client_id=test&redirect_uri=https%3A%2F%2Fexample.com&response_type=code&code_challenge="
		address := prefix + strings.Repeat("x", width-padding-len(prefix)-len("&state=abc")) + "&state=abc"
		em := vt.NewEmulator(width, height)
		_, _ = em.WriteString(address + "\r\nPress Enter after signing in.\r\n")
		link := findClaudeAuthorization(width, height, em.CellAt)
		_ = em.Close()
		if link.url != address || link.contains(0, 1) {
			t.Fatalf("padding %d: link included the instructions: %+v", padding, link)
		}
	}
}
