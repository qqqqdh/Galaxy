package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectSearchUsesBufferAndJumps(t *testing.T) {
	root := t.TempDir()
	for path, text := range map[string]string{
		"src/계산.c": "old only\n", "readme.txt": "\ufeffintro\r\n한글 marker\r\n",
		"ignored/skip.c": "marker", "binary": "marker\x00",
	} {
		file := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(file), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(text), 0644); err != nil {
			t.Fatal(err)
		}
	}
	i := newIDE(root, config{IndentSize: 4, Ignore: []string{"ignored"}})
	if err := i.openFile("src/계산.c"); err != nil {
		t.Fatal(err)
	}
	i.active.editor.SetText("one\ntwo\n한글 marker", true)
	i.newFile()
	i.active.editor.SetText("marker", true)
	project := i.projectFiles(true)
	ctx := context.Background()
	hits, limited, err := project.search(ctx, "marker")
	if err != nil || limited || len(hits) != 3 {
		t.Fatalf("%+v %v", hits, err)
	}
	for n, row := range []int{3, 1, 2} {
		h := hits[n]
		if h.line != row {
			t.Fatalf("unexpected hit %+v", h)
		}
		i.jumpToHit(h, "marker")
		selected, _, _ := i.active.editor.GetSelection()
		if selected != "marker" || i.active.disk.path != h.file.path {
			t.Fatalf("bad jump %+v %q", h, selected)
		}
	}
	old, _, err := project.search(ctx, "old only")
	if len(old) != 0 || err != nil {
		t.Fatal("searched saved text behind dirty tab")
	}
}
