package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitChangesAndDiff(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is unavailable")
	}
	dir := t.TempDir()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	ctx := context.Background()
	run := func(args ...string) {
		t.Helper()
		if _, _, err := readGit(ctx, dir, false, args...); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	run("init", "-q")
	name := "한 글[1].txt"
	write(name, "original\n")
	write("old.txt", "rename me\n")
	write("deleted.txt", "gone\n")
	run("add", "--", ".")
	run("-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "-qm", "base")
	run("mv", "old.txt", "new name.txt")
	write(name, "stage\n")
	run("add", "--", name)
	write(name, "++work\n")
	write("new file.txt", "fresh\n")
	if err := os.Remove(filepath.Join(dir, "deleted.txt")); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0755); err != nil {
		t.Fatal(err)
	}
	root, changes, err := gitChanges(ctx, sub)
	if err != nil || len(changes) != 5 {
		t.Fatalf("%+v %v", changes, err)
	}
	expected := map[string][]string{
		name + "스테이징":      {"[#ff9292]-original[-]", "[#80dca0]+stage[-]"},
		name + "작업 트리":     {"[#ff9292]-stage[-]", "[#80dca0]+++work[-]"},
		"deleted.txt작업 트리": {"[#ff9292]-gone[-]"},
		"new name.txt스테이징": {"rename from old.txt", "rename to new name.txt"},
		"new file.txt새 파일": {"[#80dca0]+fresh[-]"},
	}
	for _, c := range changes {
		key := c.path + c.stage
		want, ok := expected[key]
		if !ok {
			t.Fatal(c)
		}
		s, err := c.diff(ctx, root)
		if err != nil {
			t.Fatal(err)
		}
		for _, fragment := range want {
			if !strings.Contains(s, fragment) {
				t.Fatalf("%s missing %s in %s", key, fragment, s)
			}
		}
		delete(expected, key)
	}
}
