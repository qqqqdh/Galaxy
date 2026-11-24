package main

import (
	"net/url"
	"strings"

	uv "github.com/charmbracelet/ultraviolet"
)

type terminalLink struct {
	url                   string
	firstRow, firstColumn int
	lastRow, lastColumn   int
}

func (l terminalLink) contains(col, row int) bool {
	return l.url != "" && row >= l.firstRow && row <= l.lastRow &&
		(row != l.firstRow || col >= l.firstColumn) && (row != l.lastRow || col < l.lastColumn)
}

func findClaudeAuthorization(width, height int, cellAt func(int, int) *uv.Cell) terminalLink {
	line := make([]byte, width)
	lineAt := func(row int) string {
		for col := range line {
			line[col] = ' '
			if cell := cellAt(col, row); cell != nil && len(cell.Content) == 1 {
				if b := cell.Content[0]; b > ' ' && b < 127 {
					line[col] = b
				}
			}
		}
		return string(line)
	}
	for row := 0; row < height; row++ {
		text := lineAt(row)
		start := strings.Index(text, "https://claude.com/cai/oauth/authorize?")
		if start < 0 {
			start = strings.Index(text, "https://claude.ai/oauth/authorize?")
		}
		if start < 0 {
			continue
		}
		first, _, _ := strings.Cut(text[start:], " ")
		var address strings.Builder
		address.WriteString(first)
		endRow, endCol := row, start+len(first)
		// Claude Code hard-wraps the URL at the pane edge before writing it to the PTY.
		for endCol >= width-1 && endRow+1 < height {
			next := strings.TrimRight(lineAt(endRow+1), " ")
			if next == "" || strings.ContainsRune(next, ' ') || strings.HasPrefix(next, "https://") {
				break
			}
			endRow, endCol = endRow+1, len(next)
			address.WriteString(next)
		}
		if endRow == height-1 && endCol >= width-1 {
			continue
		}
		if completeClaudeAuthorization(address.String()) {
			return terminalLink{address.String(), row, start, endRow, endCol}
		}
	}
	return terminalLink{}
}

func completeClaudeAuthorization(address string) bool {
	parsed, err := url.ParseRequestURI(address)
	if err != nil {
		return false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	return err == nil && query.Get("client_id") != "" && query.Get("redirect_uri") != "" &&
		query.Get("response_type") == "code" && query.Get("code_challenge") != "" && query.Get("state") != ""
}
