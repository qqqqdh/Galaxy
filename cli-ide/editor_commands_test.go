package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestGoSymbolServerReferencesAndRename(t *testing.T) {
	if _, err := exec.LookPath("gopls"); err != nil {
		t.Skip("gopls is unavailable")
	}
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	a, b := filepath.Join(i.root, "declare.go"), filepath.Join(i.root, "use.go")
	source := "package sample\nvar target = 1\n"
	for path, text := range map[string]string{a: source, b: "package sample\nvar use = target\n", filepath.Join(i.root, "go.mod"): "module example.com/sample\n\ngo 1.24.2\n"} {
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := i.openFile(a); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	var client languageClient
	defer client.close()
	files := map[string]string{a: source}
	position := lspPosition(source, strings.Index(source, "target"))
	project := nativeProject{root: i.root}
	refs, err := client.query(ctx, ctx, project, files, a, position, "references")
	if err != nil {
		t.Fatal(err)
	}
	i.showReferences(refs)
	list, ok := i.app.GetFocus().(*tview.List)
	if !ok || list.GetItemCount() != 2 {
		t.Fatalf("expected declaration and usage, got %s", refs)
	}
	for n := 0; n < list.GetItemCount(); n++ {
		label, _ := list.GetItemText(n)
		if strings.HasPrefix(label, "use.go:") {
			list.GetItemSelectedFunc(n)()
			break
		}
	}
	if i.active.disk.path != b || i.active.syntax.definitionSelection == nil {
		t.Fatal("reference did not open and select the use site")
	}
	data, err := client.query(ctx, ctx, project, files, a, position, "rename", "renamed")
	if err != nil {
		t.Fatal(err)
	}
	plans, err := planRename(i.root, data, files)
	if err != nil || len(plans) != 2 {
		t.Fatalf("gopls rename plan = %v, %v; response %s", plans, err, data)
	}
}

func TestWorkspaceRenamePreviewAndApply(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	a, b := filepath.Join(i.root, "a.go"), filepath.Join(i.root, "b.go")
	original := "package p\n// 😀 한글\nvar target = 1\n"
	before := "package p\nvar use = target\n"
	for path, text := range map[string]string{a: original, b: before} {
		if err := os.WriteFile(path, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := i.openFile(a); err != nil {
		t.Fatal(err)
	}
	i.active.editor.Replace(len(original), len(original), "// kept in buffer\n")
	snapshot := i.active.editor.GetText()
	change := func(text string) lspTextEdit {
		start := strings.Index(text, "target")
		return lspTextEdit{Range: &lspRange{lspPosition(text, start), lspPosition(text, start+6)}, NewText: "renamed"}
	}
	response, err := json.Marshal(rpcParams{"documentChanges": []rpcParams{
		{"textDocument": rpcParams{"uri": fileURI(a), "version": 3}, "edits": []lspTextEdit{change(snapshot)}},
		{"textDocument": rpcParams{"uri": fileURI(b), "version": nil}, "edits": []lspTextEdit{change(before)}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRename(response, func(string, int) bool { return false }); err == nil {
		t.Fatal("accepted a stale document version")
	}
	edits, err := decodeRename(response, func(path string, version int) bool { return path == a && version == 3 })
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(edits)
	if err != nil {
		t.Fatal(err)
	}
	plans, err := planRename(i.root, data, map[string]string{a: snapshot})
	if err != nil || len(plans) != 2 {
		t.Fatalf("rename plan = %v, %v", plans, err)
	}
	if err := os.WriteFile(b, []byte(before+"// external\n"), 0644); err != nil {
		t.Fatal(err)
	}
	i.applyRename(plans)
	if i.active.editor.GetText() != snapshot || len(i.documents) != 1 {
		t.Fatal("changed buffers despite a stale preview")
	}
	if err := os.WriteFile(b, []byte(before), 0644); err != nil {
		t.Fatal(err)
	}
	i.applyRename(plans)
	if len(i.documents) != 2 || i.active.disk.path != a {
		t.Fatal("rename did not open the second file and restore the current tab")
	}
	for _, d := range i.documents {
		if !d.dirty() || strings.Contains(d.editor.GetText(), "target") || !strings.Contains(d.editor.GetText(), "renamed") {
			t.Fatalf("rename did not update %s", d.name())
		}
	}
	if !strings.Contains(i.active.editor.GetText(), "// kept in buffer") {
		t.Fatal("lost pre-existing unsaved edits")
	}
	if onDisk, err := os.ReadFile(b); err != nil || string(onDisk) != before {
		t.Fatalf("rename unexpectedly wrote to disk: %q, %v", onDisk, err)
	}
}

func TestCommandPaletteAndErrorNavigation(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	path := filepath.Join(i.root, "file.go")
	i.addDocument(&document{path: path, text: "abc\ndef\nghi\n"})
	active := i.active
	i.showProblems([]problem{
		{file: path, line: 3, first: 2, kind: "error"},
		{file: path, line: 1, first: 2, kind: "warning"},
		{file: path, line: 2, first: 1, kind: "error"},
	}, "")
	for _, step := range []struct{ direction, offset int }{{1, 4}, {1, 9}, {1, 4}, {-1, 9}} {
		i.nextProblem(step.direction)
		_, offset, _ := active.editor.GetSelection()
		if offset != step.offset {
			t.Fatalf("error navigation offset = %d, want %d", offset, step.offset)
		}
	}
	if i.handleKey(tcell.NewEventKey(tcell.KeyF10, 0, 0)) != nil || i.pickerField == nil {
		t.Fatal("F10 did not open the palette")
	}
	field := i.pickerField.SetText("새 파일")
	field.InputHandler()(tcell.NewEventKey(tcell.KeyEnter, 0, 0), func(p tview.Primitive) { i.app.SetFocus(p) })
	if i.dialog || i.active == active || i.active.disk.path != "" {
		t.Fatal("filtered command did not create a new file")
	}
}

func TestRunCurrentGoTestAndLocateFailure(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample_test.go")
	source := "package sample\nimport \"testing\"\nfunc TestOnly(t *testing.T) { t.Fatal(\"selected failure\") }\nfunc TestOther(t *testing.T) { t.Fatal(\"unexpected second test\") }\n"
	for file, text := range map[string]string{path: source, filepath.Join(dir, "go.mod"): "module example.com/sample\n\ngo 1.24.2\n"} {
		if err := os.WriteFile(file, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	name := goTestAt(path, source, strings.Index(source, "selected failure"))
	if name != "TestOnly" {
		t.Fatalf("function under the cursor = %q", name)
	}
	lines, err := executeGoTest(context.Background(), dir, name)
	if err == nil || !slices.ContainsFunc(lines, func(p problem) bool {
		return p.file == path && p.line == 3 && strings.Contains(p.message, "selected failure")
	}) || slices.ContainsFunc(lines, func(p problem) bool { return strings.Contains(p.message, "unexpected second test") }) {
		t.Fatalf("test output = %+v, %v", lines, err)
	}
}
