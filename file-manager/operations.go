package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const transferDir = "/mnt/galaxy-transfer"

func validName(name string) error {
	if name == "." || name == ".." || !filepath.IsLocal(name) || filepath.Base(name) != name || strings.ContainsRune(name, 0) {
		return errors.New("파일이나 폴더 이름 하나를 입력하세요")
	}
	return nil
}

func renameNew(from, to string) error {
	err := unix.Renameat2(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_NOREPLACE)
	if errors.Is(err, unix.EXDEV) {
		return errors.New("다른 파일 시스템으로 옮기려면 복사한 뒤 원본을 삭제하세요")
	}
	if err != nil {
		return &os.LinkError{Op: "이동", Old: from, New: to, Err: err}
	}
	return nil
}

func copyEntry(from, to string) error {
	info, err := os.Lstat(from)
	if err != nil {
		return err
	}
	if info.IsDir() {
		rel, err := filepath.Rel(from, to)
		if err != nil {
			return err
		}
		if rel == "." || filepath.IsLocal(rel) {
			return errors.New("폴더를 자기 안에 복사할 수 없습니다")
		}
	}
	stage, err := os.MkdirTemp(filepath.Dir(to), ".galaxy-copy-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	staged := filepath.Join(stage, "item")
	if err := copyTree(from, staged); err != nil {
		return err
	}
	return renameNew(staged, to)
}

func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(to, rel)
		switch {
		case entry.Type()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, destination)
		case entry.IsDir():
			return os.Mkdir(destination, 0755)
		case entry.Type().IsRegular():
			input, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
			if err != nil {
				return err
			}
			defer input.Close()
			info, err := input.Stat()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("일반 파일이 아닙니다: %s", path)
			}
			return writeNew(destination, input, info.Mode().Perm())
		default:
			return fmt.Errorf("복사할 수 없는 파일입니다: %s", path)
		}
	})
}

func writeNew(path string, input io.Reader, mode fs.FileMode) error {
	out, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, input)
	return errors.Join(err, out.Close())
}

func archiveBase(name string) (string, bool) {
	for _, suffix := range []string{".tar.gz", ".tgz", ".zip"} {
		if strings.HasSuffix(strings.ToLower(name), suffix) && len(name) > len(suffix) {
			return name[:len(name)-len(suffix)], true
		}
	}
	return "", false
}

func extractArchive(source, dest string) error {
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".galaxy-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if strings.HasSuffix(strings.ToLower(source), ".zip") {
		err = extractZip(source, stage)
	} else {
		err = extractTar(source, stage)
	}
	if err != nil {
		return err
	}
	return renameNew(stage, dest)
}

func putArchiveEntry(root, name string, mode fs.FileMode, input io.Reader) error {
	if !filepath.IsLocal(name) || strings.ContainsAny(name, "\\\x00") {
		return fmt.Errorf("압축 파일에 올바르지 않은 경로가 있습니다: %q", name)
	}
	for part := range strings.SplitSeq(name, "/") {
		if part == ".." {
			return fmt.Errorf("압축 파일에 상위 폴더 경로가 있습니다: %q", name)
		}
	}
	target := filepath.Join(root, name)
	if mode.IsDir() {
		return os.MkdirAll(target, 0755)
	}
	if !mode.IsRegular() {
		return fmt.Errorf("압축 파일에 링크 또는 특수 파일이 있습니다: %q", name)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return writeNew(target, input, 0644|mode.Perm()&0111)
}

func extractZip(source, root string) error {
	archive, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer archive.Close()
	for _, item := range archive.File {
		input, err := item.Open()
		if err != nil {
			return err
		}
		err = putArchiveEntry(root, item.Name, item.Mode(), input)
		if err = errors.Join(err, input.Close()); err != nil {
			return err
		}
	}
	return nil
}

func extractTar(source, root string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	for {
		item, err := archive.Next()
		if err == io.EOF {
			// Read to the end of gzip so the stream checksum is checked.
			_, err = io.Copy(io.Discard, gz)
			return err
		}
		if err != nil {
			return err
		}
		if item.Typeflag != tar.TypeDir && item.Typeflag != tar.TypeReg && item.Typeflag != tar.TypeRegA {
			return fmt.Errorf("압축 파일에 링크 또는 특수 파일이 있습니다: %q", item.Name)
		}
		if err := putArchiveEntry(root, item.Name, item.FileInfo().Mode(), archive); err != nil {
			return err
		}
	}
}
