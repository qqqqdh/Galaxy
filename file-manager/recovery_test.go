package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrashRestoresWithoutOverwriting(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	source := filepath.Join(root, "original.txt")
	writeTestFile := func(path, value string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(source, "before")
	if err := trashEntry(source); err != nil {
		t.Fatal(err)
	}
	dir, err := trashDirectory()
	if err != nil {
		t.Fatal(err)
	}
	items, err := os.ReadDir(dir)
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v, err=%v", items, err)
	}
	slot := filepath.Join(dir, items[0].Name())
	writeTestFile(source, "new file")
	if restoreTrash(slot, source) == nil {
		t.Fatal("restoring over another file succeeded")
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "new file" {
		t.Fatalf("destination changed: %q, %v", data, err)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	if err := restoreTrash(slot, source); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "before" {
		t.Fatalf("wrong restored file: %q, %v", data, err)
	}
}

func TestProjectBackupRestoresFilesAndGit(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{".git/HEAD": "ref: refs/heads/main\n", "run.sh": "#!/bin/sh\n", ".env": "EXAMPLE=1\n"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte(value), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("run.sh", filepath.Join(project, "run")); err != nil {
		t.Fatal(err)
	}
	if packBackup(project, filepath.Join(project, "inside.tar.gz")) == nil {
		t.Fatal("backup allowed inside own tree")
	}
	archive := filepath.Join(root, "project"+projectBackupSuffix)
	if err := packBackup(project, archive); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "restored")
	if err := restoreBackup(archive, destination); err != nil {
		t.Fatal(err)
	}
	if restoreBackup(archive, destination) == nil {
		t.Fatal("restore overwrote destination")
	}
	for _, name := range []string{".git/HEAD", ".env", "run"} {
		want, _ := os.ReadFile(filepath.Join(project, name))
		got, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil || string(got) != string(want) {
			t.Fatalf("%s=%q, want %q (%v)", name, got, want, err)
		}
	}
	if !validBackupLink("lib/run", "../run.sh") || validBackupLink("lib/run", "../../outside") ||
		validBackupLink("lib/run", "another/link/../../outside") {
		t.Fatal("unexpected backup link validation")
	}
}
