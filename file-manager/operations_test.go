package main

import (
	"archive/tar"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractGitArchive(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "repository.tar.gz")
	file, err := os.Create(source)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	archive := tar.NewWriter(gz)
	if err := archive.WriteHeader(&tar.Header{
		Name: "pax_global_header", Typeflag: tar.TypeXGlobalHeader,
		PAXRecords: map[string]string{"comment": "git archive commit"},
	}); err != nil {
		t.Fatal(err)
	}
	const contents = "package main\n"
	if err := archive.WriteHeader(&tar.Header{
		Name: "repository/main.go", Mode: 0644, Size: int64(len(contents)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Write([]byte(contents)); err != nil {
		t.Fatal(err)
	}
	for _, close := range []func() error{archive.Close, gz.Close, file.Close} {
		if err := close(); err != nil {
			t.Fatal(err)
		}
	}

	dest := filepath.Join(root, "out")
	if err := extractArchive(source, dest); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "repository", "main.go"))
	if err != nil || string(got) != contents {
		t.Fatalf("extracted %q: %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(dest, "pax_global_header")); !os.IsNotExist(err) {
		t.Fatalf("global metadata was materialized: %v", err)
	}
	want := filepath.Join(root, "normal-dir")
	if err := os.Mkdir(want, 0755); err != nil {
		t.Fatal(err)
	}
	actual, err := os.Stat(dest)
	if err != nil {
		t.Fatal(err)
	}
	normal, err := os.Stat(want)
	if err != nil {
		t.Fatal(err)
	}
	if actual.Mode().Perm() != normal.Mode().Perm() {
		t.Fatalf("extracted mode %o, want %o", actual.Mode().Perm(), normal.Mode().Perm())
	}
}
