package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCrashRecovery(t *testing.T) {
	root, state := t.TempDir(), t.TempDir()
	open := func() (*ide, int) {
		i := newIDE(root, config{IndentSize: 4})
		n, err := i.recoverDrafts(state)
		if err != nil {
			t.Fatal(err)
		}
		return i, n
	}
	i, _ := open()
	path := filepath.Join(root, "main.c")
	if err := os.WriteFile(path, []byte("original\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := i.openFile(path); err != nil {
		t.Fatal(err)
	}
	named := i.active
	named.editor.SetText("한abc글\n", true)
	named.editor.SetText("핫ab글!\n", true)
	i.newFile()
	i.active.editor.SetText("new unsaved\n", true)
	if _, n := open(); n != 0 {
		t.Fatal("another IDE claimed live buffers")
	}
	if err := os.WriteFile(path, []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	// A terminated process releases locks without running the close/save UI.
	for _, d := range i.documents {
		if d.draft == nil {
			t.Fatal("missing journal")
		}
		d.draft.lock.Close()
	}
	f, err := os.OpenFile(named.draft.path, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.WriteString("{"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	restored, n := open()
	if n != 2 {
		t.Fatal("recovered", n)
	}
	for _, d := range restored.documents {
		if !d.dirty() {
			t.Fatal("recovery must stay unsaved")
		}
		if d.disk.path == "" {
			if d.editor.GetText() != "new unsaved\n" {
				t.Fatal("lost untitled draft")
			}
			restored.save(d, filepath.Join(root, "new.txt"), nil)
		} else {
			if d.editor.GetText() != "핫ab글!\n" || !d.disk.crlf {
				t.Fatal("lost named draft")
			}
			if err := saveDocument(d.disk, path, d.editor.GetText(), false); !errors.Is(err, errConflict) {
				t.Fatal("lost original conflict guard", err)
			}
			d.editor.SetText("수정", true)
			d.draft.lock.Close()
		}
	}
	next, n := open()
	if n != 1 || next.active.editor.GetText() != "수정" {
		t.Fatal("saved draft revived or partial tail broke subsequent edit")
	}
	next.active.editor.SetText(next.active.disk.text, true)
	if next.active.draft != nil {
		t.Fatal("undo to base retained a draft")
	}
	if _, n := open(); n != 0 {
		t.Fatal(n)
	}
}
