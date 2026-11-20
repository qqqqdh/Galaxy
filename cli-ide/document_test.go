package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSavePreservesKoreanBOMAndCRLF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "한글.txt")
	original := []byte("\ufeff안녕하세요\r\n둘째 줄\r\n")
	if err := os.WriteFile(path, original, 0755); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := readDocument(path)
	if err != nil {
		t.Fatal(err)
	}
	if d.text != "안녕하세요\n둘째 줄\n" {
		t.Fatalf("unexpected content %q", d.text)
	}
	if err := saveDocument(d, path, d.text+"추가\n", false); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "\ufeff안녕하세요\r\n둘째 줄\r\n추가\r\n" {
		t.Fatalf("unexpected saved bytes %q", data)
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != before.Mode().Perm() {
		t.Fatalf("lost executable permissions %v", info.Mode())
	}
	leftovers, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".galaxy-save-*"))
	if len(leftovers) != 0 {
		t.Fatal("temporary save files left behind", leftovers)
	}
}

func TestSaveDetectsExternalChangesAndDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file.txt")
	os.WriteFile(path, []byte("first"), 0644)
	d, _ := readDocument(path)
	os.WriteFile(path, []byte("other"), 0644)
	if err := saveDocument(d, path, "editor", false); !errors.Is(err, errConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "other" || d.text != "first" {
		t.Fatal("conflict modified the document")
	}
	if err := saveDocument(d, path, "editor", true); err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	if err := saveDocument(d, path, "editor2", false); !errors.Is(err, errConflict) {
		t.Fatalf("expected deleted-file conflict, got %v", err)
	}
}

func TestSaveAsRequiresOverwriteAndCreatesParents(t *testing.T) {
	d := &document{mode: 0644}
	path := filepath.Join(t.TempDir(), "nested", "new.txt")
	if err := saveDocument(d, path, "new", false); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(path), "existing.txt")
	os.WriteFile(other, []byte("keep"), 0644)
	if err := saveDocument(d, other, "replace", false); !errors.Is(err, errConflict) {
		t.Fatal("save-as silently overwrote an existing file", err)
	}
	data, _ := os.ReadFile(other)
	if string(data) != "keep" {
		t.Fatal("existing file was changed")
	}
}

func TestSymlinkSavePreservesLink(t *testing.T) {
	dir := t.TempDir()
	target, link := filepath.Join(dir, "target.txt"), filepath.Join(dir, "link.txt")
	os.WriteFile(target, []byte("before"), 0644)
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlink creation unavailable", err)
	}
	d, err := readDocument(link)
	if err != nil {
		t.Fatal(err)
	}
	if err := saveDocument(d, link, "after", false); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Lstat(link)
	data, _ := os.ReadFile(target)
	if info.Mode()&os.ModeSymlink == 0 || string(data) != "after" {
		t.Fatal("saving replaced the symlink")
	}
	os.Remove(target)
	if _, err := readDocument(link); err == nil {
		t.Fatal("dangling symlink was accepted as a new file")
	}
}

func TestRejectsBinaryAndOversizeWithoutChangingFiles(t *testing.T) {
	for name, data := range map[string][]byte{
		"binary": {0, 1, 2}, "invalid": {0xff}, "large": bytes.Repeat([]byte("a"), maxFileSize+1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), name)
			os.WriteFile(path, data, 0644)
			if _, err := readDocument(path); err == nil {
				t.Fatal("unsupported file accepted")
			}
			actual, _ := os.ReadFile(path)
			if !bytes.Equal(actual, data) {
				t.Fatal("opening changed source data")
			}
		})
	}
}

func TestTaskArgumentsStaySeparate(t *testing.T) {
	file := "/tmp/a file;touch unwanted.py"
	args, err := expandArgs([]string{"python3", "{file}", "--root={root}"}, file, "/tmp/project space")
	if err != nil || len(args) != 3 || args[1] != file || args[2] != "--root=/tmp/project space" {
		t.Fatalf("arguments changed %q %v", args, err)
	}
	if _, err := expandArgs([]string{"python3", "{file}"}, "", "/tmp"); err == nil {
		t.Fatal("unsaved file task was accepted")
	}
}

func TestConfigProjectOverridesAndValidation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	root := t.TempDir()
	path := filepath.Join(root, ".galaxy-ide.json")
	os.WriteFile(path, []byte(`{"indent_size":2,"tasks":[{"name":"test","args":["go","test","./..."]}]}`), 0644)
	c, err := loadConfig(root)
	if err != nil || c.IndentSize != 2 || len(c.Tasks) != 1 {
		t.Fatalf("config not loaded %#v %v", c, err)
	}
	for _, data := range []string{`{`, `{"indent_size":0}`, `{"tasks":[{"name":"bad","args":[]}]}`,
		`{"tasks":[{"name":"a","group":"invalid","args":["make"]}]}`,
		`{"tasks":[{"name":"a","group":"build","args":["make"]},{"name":"b","group":"build","args":["go","build"]}]}`} {
		os.WriteFile(path, []byte(data), 0644)
		if _, err := loadConfig(root); err == nil {
			t.Fatalf("invalid configuration accepted %s", data)
		}
	}
}

func TestEmptyFileAndTrailingNewlineRoundTrip(t *testing.T) {
	for _, text := range []string{"", "한글", "a\n", "a\n\n", "\ufeffhello\r\n"} {
		path := filepath.Join(t.TempDir(), "file.txt")
		os.WriteFile(path, []byte(text), 0644)
		d, err := readDocument(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := saveDocument(d, path, d.text, false); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(path)
		if string(data) != text {
			t.Fatalf("round trip changed %q into %q", text, data)
		}
	}
}

func TestRejectsOversizeSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.txt")
	d := &document{mode: 0644}
	if err := saveDocument(d, path, strings.Repeat("a", maxFileSize+1), false); err == nil {
		t.Fatal("oversize save accepted")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("rejected save created a file")
	}
}
