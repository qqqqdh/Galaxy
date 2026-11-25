package main

import (
	"image"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

type sourceSwatch struct {
	start, width int
	color        tcell.Color
	edit         colorLiteral
	variable     string
}

type colorLiteral struct {
	start, end int
	rgba       [4]float64
}

func literalSwatch(start, width, length int, r, g, b, alpha float64) sourceSwatch {
	return sourceSwatch{
		start: start, width: width, color: compositeSwatch(r, g, b, alpha),
		edit: colorLiteral{start, start + length, [4]float64{r, g, b, alpha}},
	}
}

func (s *sourceSwatch) shift(n int) {
	s.start += n
	if s.edit.end > 0 {
		s.edit.start += n
		s.edit.end += n
	}
}

func appendHexSwatch(matches []sourceSwatch, source string, start int) []sourceSwatch {
	if start >= len(source) || source[start] != '#' {
		return matches
	}
	digits := source[start+1:]
	n := 0
	for n < len(digits) && n < 8 {
		c := digits[n]
		if !(c >= '0' && c <= '9' || c|32 >= 'a' && c|32 <= 'f') {
			break
		}
		n++
	}
	if n < len(digits) {
		c := digits[n]
		if c >= 0x80 || c >= '0' && c <= '9' || c|32 >= 'a' && c|32 <= 'z' || c == '_' || c == '-' || c == '\\' {
			return matches // The hash continues as a CSS identifier.
		}
	}
	digits = digits[:n]
	switch n {
	case 3, 4:
		var doubled [8]byte
		for i := range n {
			doubled[i*2], doubled[i*2+1] = digits[i], digits[i]
		}
		digits = string(doubled[:2*n])
	case 6, 8:
	default:
		return matches
	}
	value, _ := strconv.ParseUint(digits, 16, 32)
	if len(digits) == 6 {
		value = value<<8 | 0xff
	}
	return append(matches, literalSwatch(start, n+1, n+1, float64(value>>24), float64(value>>16&0xff), float64(value>>8&0xff), float64(value&0xff)/255))
}

func compositeSwatch(r, g, b, alpha float64) tcell.Color {
	bgR, bgG, bgB := background.RGB()
	blend := func(channel float64, bg int32) int32 {
		return int32(math.Round(channel*alpha + float64(bg)*(1-alpha)))
	}
	return tcell.NewRGBColor(blend(r, bgR), blend(g, bgG), blend(b, bgB))
}

const cssSpace = " \t\n\r\f"

var cssNumber = regexp.MustCompile(`^[+-]?(?:[0-9]*\.[0-9]+|[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
var cssVariableName = regexp.MustCompile(`^--[a-zA-Z_][\w-]*$`)

func rgbaComponent(text string, maximum float64, modern bool) (float64, bool) {
	text = strings.Trim(text, cssSpace)
	if modern && strings.EqualFold(text, "none") {
		return 0, true
	}
	number, percent := strings.CutSuffix(text, "%")
	if !cssNumber.MatchString(number) {
		return 0, false
	}
	value, err := strconv.ParseFloat(number, 64)
	if percent {
		return min(max(value, 0), 100) * maximum / 100, err == nil
	}
	return min(max(value, 0), maximum), err == nil
}

func appendCSSSwatch(matches []sourceSwatch, token chroma.Token, source string, start int) []sourceSwatch {
	if token.Type == chroma.LiteralNumberHex {
		return appendHexSwatch(matches, source, start)
	}
	if token.Type != chroma.NameBuiltin && token.Type != chroma.NameFunction {
		return matches
	}
	isRGBA, isVar := strings.EqualFold(token.Value, "rgba"), strings.EqualFold(token.Value, "var")
	if !isRGBA && !isVar {
		return matches
	}
	open := start + len(token.Value)
	if open >= len(source) || source[open] != '(' {
		return matches
	}
	value := source[open+1:]
	close := strings.IndexAny(value, "()")
	// shortcut: previews resolve literal components. Computed components require CSS evaluation.
	if close < 0 || value[close] != ')' {
		return matches
	}
	value = value[:close]
	if isVar {
		name := strings.Trim(value, cssSpace)
		if cssVariableName.MatchString(name) {
			return append(matches, sourceSwatch{start: start, width: len(token.Value) + 1, variable: name})
		}
		return matches
	}
	modern := !strings.ContainsRune(value, ',')
	alphaText := "1"
	var channels []string
	if modern {
		rgb, alpha, hasAlpha := strings.Cut(value, "/")
		channels = strings.FieldsFunc(rgb, func(r rune) bool { return strings.ContainsRune(cssSpace, r) })
		if hasAlpha {
			alphaText = alpha
		}
	} else {
		channels = strings.Split(value, ",")
		if len(channels) == 4 {
			alphaText = channels[3]
			channels = channels[:3]
		}
	}
	if len(channels) != 3 {
		return matches
	}
	var rgb [3]float64
	percent := strings.HasSuffix(strings.Trim(channels[0], cssSpace), "%")
	for i, channel := range channels {
		if !modern && strings.HasSuffix(strings.Trim(channel, cssSpace), "%") != percent {
			return matches
		}
		value, ok := rgbaComponent(channel, 255, modern)
		if !ok {
			return matches
		}
		rgb[i] = value
	}
	alpha, ok := rgbaComponent(alphaText, 1, modern)
	if !ok {
		return matches
	}
	return append(matches, literalSwatch(start, len(token.Value)+1, len(token.Value)+close+2, rgb[0], rgb[1], rgb[2], alpha))
}

func recordCSSVariable(vars map[string]sourceSwatch, token chroma.Token, source string, offset, base int) {
	if token.Type != chroma.NameVariable || !cssVariableName.MatchString(token.Value) {
		return
	}
	if _, duplicate := vars[token.Value]; duplicate {
		// Without selector matching a second declaration is ambiguous.
		vars[token.Value] = sourceSwatch{}
		return
	}
	remainder := source[min(len(source), offset+len(token.Value)):]
	remainder = strings.TrimLeft(remainder, cssSpace)
	if !strings.HasPrefix(remainder, ":") {
		return
	}
	value := strings.TrimLeft(remainder[1:], cssSpace)
	start := len(source) - len(value)
	var matches []sourceSwatch
	switch {
	case strings.HasPrefix(value, "#"):
		matches = appendHexSwatch(nil, source, start)
	case len(value) >= 5 && strings.EqualFold(value[:5], "rgba("):
		matches = appendCSSSwatch(nil, chroma.Token{Type: chroma.NameBuiltin, Value: value[:4]}, source, start)
	case len(value) >= 4 && strings.EqualFold(value[:4], "var("):
		matches = appendCSSSwatch(nil, chroma.Token{Type: chroma.NameFunction, Value: value[:3]}, source, start)
	}
	vars[token.Value] = sourceSwatch{}
	if len(matches) == 0 {
		return
	}
	match := matches[0]
	end := match.edit.end
	if match.variable != "" {
		end = start + strings.IndexByte(value, ')') + 1
	}
	tail := strings.TrimLeft(source[end:], cssSpace)
	if tail == "" || tail[0] == ';' || tail[0] == '}' || strings.HasPrefix(tail, "!important") {
		match.shift(base)
		vars[token.Value] = match
	}
}

func htmlSwatches(matches []sourceSwatch, token chroma.Token, attr string, start int, vars map[string]sourceSwatch) ([]sourceSwatch, string) {
	switch {
	case token.Type == chroma.NameAttribute:
		return matches, strings.ToLower(strings.TrimSpace(token.Value))
	case token.Type == chroma.Text && strings.TrimSpace(token.Value) == "", token.Type == chroma.Operator && token.Value == "=":
		return matches, attr
	case token.Type == chroma.LiteralString && attr != "":
		value := token.Value
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[0] == value[len(value)-1] {
			value = value[1 : len(value)-1]
			start++
		}
		switch attr {
		case "style":
			css := lexers.Get("css")
			next, err := css.Tokenise(&chroma.TokeniseOptions{State: "content", Nested: true}, value)
			if err == nil {
				offset := 0
				for part := next(); part != chroma.EOF; part = next() {
					recordCSSVariable(vars, part, value, offset, start)
					before := len(matches)
					matches = appendCSSSwatch(matches, part, value, offset)
					if len(matches) > before {
						matches[before].shift(start)
					}
					offset += len(part.Value)
				}
			}
		case "color", "bgcolor", "fill", "stroke", "stop-color", "flood-color", "lighting-color":
			trimmed := strings.TrimSpace(value)
			matches = appendHexSwatch(matches, trimmed, 0)
			if len(matches) > 0 {
				if matches[0].width != len(trimmed) {
					return matches[:0], ""
				}
				matches[0].shift(start + strings.Index(value, trimmed))
			}
		}
	}
	return matches, ""
}

type colorSwatch struct {
	colorSpan
	lineEnd  int
	edit     colorLiteral
	variable string
}

type swatchCell struct {
	image.Point
	color tcell.Color
	edit  colorLiteral
}

func (v *syntaxView) drawColorSwatches(screen *editorScreen) {
	x, y, width, height := v.GetInnerRect()
	row, column := v.GetOffset()
	right := x + width
	first := sort.Search(len(v.swatches), func(i int) bool { return v.swatches[i].row >= row })
	line, inserted, beforeCursor := -1, 0, 0
	for _, s := range v.swatches[first:] {
		if s.row >= row+height {
			break
		}
		if s.row != line {
			line, inserted, beforeCursor = s.row, 0, 0
		}
		if s.start < column || s.end+inserted+1 > column+width {
			continue
		}
		// Keep the end of a fully visible line and the editing cursor in view.
		if s.lineEnd <= column+width && s.lineEnd+inserted+1 > column+width {
			continue
		}
		if screen.cursorVisible && y+s.row-row == screen.cursorY && x+s.start-column <= screen.cursorX {
			if screen.cursorX+beforeCursor+1 >= right {
				continue
			}
			beforeCursor++
		}
		point := image.Pt(x+s.start-column+inserted, y+s.row-row)
		v.drawnSwatches = append(v.drawnSwatches, swatchCell{point, s.color, s.edit})
		inserted++
	}

	// Copy the original cells before inserting swatches, including selection and wide glyphs.
	type textCell struct {
		main      rune
		combining []rune
		style     tcell.Style
		width     int
	}
	var cells []textCell
	for first := 0; first < len(v.drawnSwatches); {
		start := v.drawnSwatches[first].Point
		end := first + 1
		for end < len(v.drawnSwatches) && v.drawnSwatches[end].Y == start.Y {
			end++
		}
		cells = cells[:0]
		for col := start.X; col < right; {
			r, combining, style, _ := screen.GetContent(col, start.Y)
			w := max(1, uniseg.StringWidth(string(r)+string(combining)))
			cells = append(cells, textCell{r, combining, style, w})
			col += w
		}
		col := start.X
		for _, c := range cells {
			if first < end && v.drawnSwatches[first].X == col {
				chip := v.drawnSwatches[first]
				glyph, color, chipStyle := '■', chip.color, c.style
				_, bg, _ := chipStyle.Decompose()
				if color == bg {
					glyph, color = '□', muted
				} else {
					r, g, b := color.RGB()
					bgR, bgG, bgB := bg.RGB()
					if max(r-bgR, bgR-r, g-bgG, bgG-g, b-bgB, bgB-b) < 32 {
						chipStyle = chipStyle.Background(muted)
					}
				}
				screen.SetContent(col, start.Y, glyph, nil, chipStyle.Foreground(color))
				first++
				col++
			}
			if col+c.width > right {
				if col < right {
					screen.SetContent(col, start.Y, ' ', nil, c.style)
				}
				break
			}
			screen.SetContent(col, start.Y, c.main, c.combining, c.style)
			col += c.width
		}
		first = end
	}
	if screen.cursorVisible {
		screen.ShowCursor(v.decoratedColumn(screen.cursorX, screen.cursorY), screen.cursorY)
	}
}

func (v *syntaxView) decoratedColumn(x, y int) int {
	for _, chip := range v.drawnSwatches {
		if chip.Y == y && chip.X <= x {
			x++
		}
	}
	return x
}

func (v *syntaxView) sourceColumn(x, y int) int {
	if len(v.drawnSwatches) == 0 {
		return x
	}
	left, _, width, _ := v.GetInnerRect()
	if x < left || x >= left+width {
		return x
	}
	column := x
	for _, chip := range v.drawnSwatches {
		if chip.Y == y && chip.X < x {
			column--
		}
	}
	return column
}
