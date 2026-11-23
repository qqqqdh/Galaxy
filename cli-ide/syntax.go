package main

import (
	"path/filepath"
	"sort"
	"sync/atomic"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

// The text area owns the document, selection, cursor and input. Only recolor its drawn cells.
type syntaxView struct {
	*tview.TextArea
	definitionSelection *[2]int
	app                 *tview.Application
	filename            string
	lexer               chroma.Lexer
	revision            atomic.Uint64
	valid               bool
	working             bool
	tabSize             int
	spans               []colorSpan
	marks               []colorSpan
	columns             int
	columnsRevision     uint64
	columnsTabSize      int
	scrollDrag          bool
	scrollX             int
}

type colorSpan struct {
	row, start, end int
	color           tcell.Color
}

func (v *syntaxView) setFilename(path string) {
	if v.filename == path {
		return
	}
	v.filename = path
	v.lexer = lexers.Match(filepath.Base(path))
	v.invalidate()
}

func (v *syntaxView) invalidate() {
	v.revision.Add(1)
	v.valid = false
	v.spans = nil
}

func (v *syntaxView) Draw(screen tcell.Screen) {
	v.TextArea.Draw(screen)
	// A newly opened target needs its line layout before byte offsets map to screen rows.
	if r := v.definitionSelection; r != nil {
		v.definitionSelection = nil
		v.Select(r[0], r[1])
		v.TextArea.Draw(screen)
	}
	v.drawColumnBar(screen)
	if v.lexer == nil || v.GetTextLength() == 0 {
		return
	}
	if v.tabSize != tview.TabSize {
		v.tabSize = tview.TabSize
		v.invalidate()
	}
	if !v.valid && !v.working {
		source, lexer, tabSize, revision := v.GetText(), v.lexer, v.tabSize, v.revision.Load()
		if len(source) <= 4096 {
			v.spans = syntaxSpans(lexer, source, tabSize, nil)
			v.valid = true
		} else {
			// Keep typing responsive while lexing a larger document.
			v.working = true
			go func() {
				stale := func() bool { return v.revision.Load() != revision }
				spans := syntaxSpans(lexer, source, tabSize, stale)
				v.app.QueueUpdateDraw(func() {
					v.working = false
					if !stale() {
						v.spans, v.valid = spans, true
					}
				})
			}()
		}
	}
	x, y, width, height := v.GetInnerRect()
	row, column := v.GetOffset()
	base := v.GetTextStyle()
	first := sort.Search(len(v.spans), func(i int) bool { return v.spans[i].row >= row })
	for _, s := range v.spans[first:] {
		if s.row >= row+height {
			break
		}
		screenY := y + s.row - row
		for col := max(s.start, column); col < min(s.end, column+width); {
			screenX := x + col - column
			r, combining, style, cellWidth := screen.GetContent(screenX, screenY)
			if style == base && r > ' ' {
				screen.SetContent(screenX, screenY, r, combining, style.Foreground(s.color))
			}
			col += max(1, cellWidth)
		}
	}
	for _, m := range v.marks {
		if m.row < row || m.row >= row+height {
			continue
		}
		for col := max(column, m.start); col < min(column+width, m.end); {
			sx, sy := x+col-column, y+m.row-row
			r, comb, style, w := screen.GetContent(sx, sy)
			screen.SetContent(sx, sy, r, comb, style.Foreground(m.color).Underline(true))
			col += max(1, w)
		}
	}
}

var syntaxColors = map[chroma.TokenType]tcell.Color{
	chroma.Keyword:            tcell.NewHexColor(0xcba6f7),
	chroma.KeywordType:        tcell.NewHexColor(0x89dceb),
	chroma.CommentPreproc:     tcell.NewHexColor(0xcba6f7),
	chroma.CommentPreprocFile: tcell.NewHexColor(0xa6e3a1),
	chroma.Comment:            tcell.NewHexColor(0x93a4b8),
	chroma.String:             tcell.NewHexColor(0xa6e3a1),
	chroma.Number:             tcell.NewHexColor(0xfab387),
	chroma.NameFunction:       tcell.NewHexColor(0x89b4fa),
	chroma.NameBuiltin:        tcell.NewHexColor(0x89b4fa),
	chroma.NameClass:          tcell.NewHexColor(0x89dceb),
	chroma.NameTag:            tcell.NewHexColor(0xcba6f7),
	chroma.NameAttribute:      tcell.NewHexColor(0xf9e2af),
}

func tokenColor(kind chroma.TokenType) tcell.Color {
	if c, ok := syntaxColors[kind]; ok {
		return c
	}
	if c, ok := syntaxColors[kind.SubCategory()]; ok {
		return c
	}
	return syntaxColors[kind.Category()]
}

func syntaxSpans(lexer chroma.Lexer, source string, tabSize int, stale func() bool) (spans []colorSpan) {
	// Chroma iterators can panic on lexer errors. The underlying plain text stays editable.
	defer func() {
		if recover() != nil {
			spans = nil
		}
	}()
	next, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root"}, source)
	if err != nil {
		return nil
	}
	clusters := uniseg.NewGraphemes(source)
	row, column, consumed, tokenEnd := 0, 0, 0, 0
	for token := next(); token != chroma.EOF; token = next() {
		if stale != nil && stale() {
			return nil
		}
		tokenEnd += len(token.Value)
		color := tokenColor(token.Type)
		for consumed < tokenEnd && clusters.Next() {
			part := clusters.Str()
			consumed += len(part)
			if part == "\n" || part == "\r\n" {
				row++
				column = 0
				continue
			}
			width := clusters.Width()
			if part == "\t" {
				width = tabSize
			}
			if color != tcell.ColorDefault && width > 0 {
				last := len(spans) - 1
				if last >= 0 && spans[last].row == row && spans[last].end == column && spans[last].color == color {
					spans[last].end += width
				} else {
					spans = append(spans, colorSpan{row, column, column + width, color})
				}
			}
			column += width
		}
	}
	return spans
}
