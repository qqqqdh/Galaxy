package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestEditorColorSwatches(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(100, 14)

	i := newIDE(t.TempDir(), config{IndentSize: 4})
	i.newFile()
	d := i.active
	v := d.syntax
	v.SetRect(0, 0, 100, 13)
	css := "#fff {\n\tcolor:#F60; --shadow:#10203080; border-color:#1234 #112233; fill:#0000;\n" +
		"\tbackground:url(#abc); content:'#abc'; /* #abc */\n" +
		"\tcolor:#fffffff; background:#123456789; outline-color:#abc-de;\n" +
		"\t--shadow: 0 10px 25px rgba(15, 23, 42, 0.08);\n" +
		"\tcolor: RGBA(255, 0, 0, .5); fill: rgba(100% 0 50% / 50%);\n" +
		"\tcolor: rgba(300 -5 none / 120%);\n" +
		"\tcolor: rgba(100%, 0, 0, 1); content:'rgba(1,2,3,1)'; /* rgba(1,2,3,1) */\n}"
	html := "<style>p { color: #123abc; }</style>\n" +
		"<p style=\"color: #abc;\n background: #456def\" bgcolor='#246' id='#aaa'>#abc</p>\n" +
		"<a href=\"#abc\">#abc</a><!-- #fff -->\n" +
		"<style>p { color: rgba(51, 102, 153, 1); }</style>\n" +
		"<p style='color: rgba(10% 20% 30% / 100%)'></p>\n" +
		"<script>const x = rgba(255, 0, 0, 1);</script>"
	for _, tc := range []struct {
		filename string
		source   string
		want     []int32
	}{
		{"colors.css", css, []int32{0xff6600, 0x101c29, 0x111a27, 0x112233, background.Hex(), 0x111723, 0x880c11, 0x880c51, 0xff0000}},
		{"colors.html", html, []int32{0x123abc, 0xaabbcc, 0x456def, 0x224466, 0x336699, 0x1a334d}},
		{"plain.txt", css + html, nil},
	} {
		t.Run(tc.filename, func(t *testing.T) {
			d.editor.SetText(tc.source, false)
			v.setFilename(tc.filename)
			v.Draw(screen)
			var got []int32
			for _, chip := range v.drawnSwatches {
				r, _, style, _ := screen.GetContent(chip.X, chip.Y)
				fg, bg, _ := style.Decompose()
				next, _, _, _ := screen.GetContent(chip.X+1, chip.Y)
				wantGlyph, wantFg := '■', chip.color
				if chip.color == background {
					wantGlyph, wantFg = '□', muted
				}
				if r != wantGlyph || fg != wantFg || !strings.ContainsRune("#rR", next) {
					t.Fatalf("want square directly before color code, got %q%q at %v", r, next, chip.Point)
				}
				if chip.color.Hex() == 0x111723 && bg != muted {
					t.Fatal("the near-background shadow swatch needs a contrasting cell")
				}
				got = append(got, chip.color.Hex())
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("swatches = %06x, want %06x", got, tc.want)
			}
		})
	}
}

func TestColorSwatchEditing(t *testing.T) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(75, 10)

	root := t.TempDir()
	i := newIDE(root, config{IndentSize: 4})
	i.newFile()
	d := i.active
	v := d.syntax
	v.setFilename("colors.css")
	d.editor.SetText("p { --한글: 1; color: #abc; content: '한글'; background: #246; }", false)
	v.SetRect(0, 0, 75, 9)
	i.app.SetFocus(d.editor)
	v.Draw(screen)
	if len(v.drawnSwatches) != 2 {
		t.Fatalf("got %d swatches", len(v.drawnSwatches))
	}
	first, last := v.drawnSwatches[0], v.drawnSwatches[1]
	handle := v.MouseHandler()
	click := func(action tview.MouseAction, x int) tview.Primitive {
		_, capture := handle(action, tcell.NewEventMouse(x, first.Y, tcell.Button1, tcell.ModNone), func(p tview.Primitive) { i.app.SetFocus(p) })
		return capture
	}
	if capture := click(tview.MouseLeftDown, last.X+1); capture != v {
		t.Fatal("drag capture bypassed the decorated editor")
	}
	click(tview.MouseMove, first.X)
	click(tview.MouseLeftUp, first.X)
	selection, start, _ := d.editor.GetSelection()
	if selection != "#abc; content: '한글'; background: " {
		t.Fatalf("reverse drag selected %q", selection)
	}
	v.Draw(screen)
	if x, y, visible := screen.GetCursor(); !visible || x != first.X+1 || y != first.Y {
		t.Fatalf("cursor = (%d, %d, %v), want (%d, %d, true)", x, y, visible, first.X+1, first.Y)
	}
	_, _, style, _ := screen.GetContent(first.X+1, first.Y)
	if fg, _, _ := style.Decompose(); fg != tcell.ColorWhite {
		t.Fatal("decoration covered the selection style")
	}
	d.editor.Replace(start, start+4, "#f50")
	v.Draw(screen)
	if v.drawnSwatches[0].color.Hex() != 0xff5500 {
		t.Fatal("editing did not update the swatch")
	}
	v.SetOffset(0, 30)
	v.Draw(screen)
	if len(v.drawnSwatches) != 1 || v.drawnSwatches[0].color.Hex() != 0x224466 {
		t.Fatal("horizontal scroll did not update the visible swatches")
	}
	path := filepath.Join(root, "colors.css")
	i.save(d, path, nil)
	data, err := os.ReadFile(path)
	if err != nil || string(data) != d.editor.GetText() || strings.ContainsRune(string(data), '■') || !strings.Contains(string(data), "#f50") {
		t.Fatalf("unexpected saved source: %q, %v", data, err)
	}
}
