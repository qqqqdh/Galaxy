package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestWebPreviewRefreshesAfterSave(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	file := filepath.Join(i.root, "index.html")
	if err := os.WriteFile(file, []byte("<p>before</p>"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := i.openFile(file); err != nil {
		t.Fatal(err)
	}
	preview, err := startWebPreview(i.root)
	if err != nil {
		t.Fatal(err)
	}
	defer preview.close()
	i.preview = preview
	client := &http.Client{Timeout: 5 * time.Second}
	read := func(path string) string {
		t.Helper()
		res, err := client.Get(preview.base + path)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		data, err := io.ReadAll(res.Body)
		if err != nil || res.StatusCode != http.StatusOK || res.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("preview response %d, %v", res.StatusCode, err)
		}
		return string(data)
	}
	if page := read("/"); !strings.Contains(page, "<p>before</p>") || !strings.Contains(page, previewEvents+"?since=0") {
		t.Fatal(page)
	}
	stream, err := client.Get(preview.base + previewEvents + "?since=0")
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	i.active.editor.SetText("<p>after</p>", false)
	i.save(i.active, "", nil)
	event, err := io.ReadAll(stream.Body)
	if err != nil || !strings.Contains(string(event), "event: reload") {
		t.Fatalf("reload stream = %q, %v", event, err)
	}
	if page := read("/"); !strings.Contains(page, "<p>after</p>") || !strings.Contains(page, previewEvents+"?since=1") {
		t.Fatal(page)
	}
}

func TestProjectSessionRestoresOpenTabsAndPosition(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	first := newIDE(root, config{IndentSize: 4})
	fileA, fileB := filepath.Join(first.root, "a.txt"), filepath.Join(first.root, "b.txt")
	text := strings.Repeat("abcdefghijklmnopqrstuvwxyz\n", 10)
	for _, file := range []string{fileA, fileB} {
		if err := os.WriteFile(file, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := first.restoreSession(state); err != nil {
		t.Fatal(err)
	}
	if err := first.openFile(fileA); err != nil {
		t.Fatal(err)
	}
	if err := first.openFile(fileB); err != nil {
		t.Fatal(err)
	}
	first.activate(first.documents[0])
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	first.active.syntax.SetRect(0, 0, 24, 8)
	first.active.syntax.Draw(screen)
	first.active.editor.Select(69, 69)
	first.active.syntax.SetOffset(1, 3)
	if err := first.saveSession(); err != nil {
		t.Fatal(err)
	}
	second := newIDE(root, config{IndentSize: 4})
	if err := second.restoreSession(state); err != nil {
		t.Fatal(err)
	}
	if len(second.documents) != 2 || second.active.disk.path != fileA || second.documents[1].disk.path != fileB {
		t.Fatal("session did not preserve tabs or active file")
	}
	second.active.syntax.SetRect(0, 0, 24, 8)
	second.active.syntax.Draw(screen)
	_, offset, _ := second.active.editor.GetSelection()
	row, col := second.active.syntax.GetOffset()
	if row != 1 || col != 3 || offset != 69 {
		t.Fatalf("restored row/column/caret = %d/%d/%d", row, col, offset)
	}
}

func TestCSSVariableSwatchOpensColorEditor(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	i.addDocument(&document{path: filepath.Join(i.root, "colors.css"), text: ".sample { color: var(--accent); }\n:root { --accent: #2563eb; }"})
	v := i.active.syntax
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	v.SetRect(0, 0, 70, 8)
	v.Draw(screen)
	if len(v.drawnSwatches) != 2 || v.drawnSwatches[0].color.Hex() != 0x2563eb || v.drawnSwatches[0].edit != v.drawnSwatches[1].edit {
		t.Fatalf("variable swatches = %+v", v.drawnSwatches)
	}
	chip := v.drawnSwatches[0]
	v.MouseHandler()(tview.MouseLeftClick, tcell.NewEventMouse(chip.X, chip.Y, tcell.ButtonNone, 0), func(p tview.Primitive) { i.app.SetFocus(p) })
	field, ok := i.app.GetFocus().(*tview.InputField)
	if !i.dialog || !ok || field.GetText() != "#2563eb" {
		t.Fatal("tapping the CSS variable swatch did not open its declaration's color editor")
	}
	if got := formatPickedColor("#2563eb", [4]float64{255, 102, 0, .5}); got != "#ff660080" {
		t.Fatal(got)
	}
	if got := formatPickedColor("rgba(1, 2, 3, .5)", [4]float64{255, 102, 0, .5}); got != "rgba(255, 102, 0, 0.5)" {
		t.Fatal(got)
	}
}
