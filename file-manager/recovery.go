package main

import (
	"archive/tar"
	"compress/gzip"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"galaxy.local/tools/ui"
	"golang.org/x/sys/unix"
)

const projectBackupSuffix = ".galaxy-backup.tar.gz"

func trashDirectory() (string, error) {
	data := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(data) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		data = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(data, "galaxy-files", "trash"), nil
}

func inside(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && filepath.IsLocal(rel)
}

func trashEntry(source string) error {
	dir, err := trashDirectory()
	if err != nil {
		return err
	}
	if inside(source, dir) || inside(dir, source) {
		return errors.New("휴지통과 그 상위 폴더는 휴지통에 넣을 수 없습니다")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	slot, err := os.MkdirTemp(dir, "item-")
	if err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(slot, "original"), []byte(source), 0600); err == nil {
		err = renameNew(source, filepath.Join(slot, "file"))
	}
	if err != nil {
		_ = os.RemoveAll(slot)
	}
	return err
}

func restoreTrash(slot, original string) error {
	parent, err := filepath.EvalSymlinks(filepath.Dir(original))
	if err != nil {
		return fmt.Errorf("원래 폴더를 다시 만든 후 복원하세요: %w", err)
	}
	if parent != filepath.Dir(original) {
		return errors.New("원래 폴더가 링크로 바뀌었습니다")
	}
	if err := renameNew(filepath.Join(slot, "file"), original); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(slot, "original"))
	_ = os.Remove(slot)
	return nil
}

func (m *fileManager) trash() {
	if m.busy {
		return
	}
	dir, err := trashDirectory()
	if err != nil {
		m.message(err.Error())
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		m.message(err.Error())
		return
	}
	var actions []ui.Action
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "item-") {
			continue
		}
		slot := filepath.Join(dir, entry.Name())
		original, err := os.ReadFile(filepath.Join(slot, "original"))
		if err != nil || !filepath.IsAbs(string(original)) || filepath.Clean(string(original)) != string(original) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(slot, "file")); err != nil {
			continue
		}
		actions = append(actions, ui.Action{Label: ui.Text(string(original)), Run: func() {
			m.Menu(ui.Text(filepath.Base(string(original))),
				ui.Action{Label: "원래 위치에 복원", Run: func() {
					m.perform("휴지통에서 복원", filepath.Base(string(original)), func() error {
						return restoreTrash(slot, string(original))
					}, nil)
				}},
				ui.Action{Label: "영구 삭제", Run: func() {
					m.Confirm(ui.Text(string(original))+" 항목을 영구 삭제할까요?", func() {
						m.perform("영구 삭제", "", func() error { return os.RemoveAll(slot) }, nil)
					})
				}},
			)
		}})
	}
	if len(actions) == 0 {
		m.message("휴지통이 비어 있습니다.")
		return
	}
	m.Menu("휴지통", actions...)
}

func (m *fileManager) backup(source string) {
	name := filepath.Base(source) + "-" + time.Now().Format("20060102-150405") + projectBackupSuffix
	start := func(directory string, android bool) {
		dir, err := filepath.EvalSymlinks(directory)
		if err != nil {
			m.message(err.Error())
			return
		}
		if strings.ContainsAny(name, "\\\r\n\t") {
			m.message("백업에 사용할 폴더 이름을 확인하세요.")
			return
		}
		m.perform("프로젝트 백업", name, func() error {
			return packBackup(source, filepath.Join(dir, name))
		}, func() {
			if android {
				fmt.Fprintf(os.Stdout, "\x1b]778;export;%s\x07", base64.StdEncoding.EncodeToString([]byte(name)))
			}
		})
	}
	options := []ui.Action{{Label: "Ubuntu 폴더에 백업", Run: func() {
		m.Prompt("백업할 위치", "폴더", filepath.Dir(source), func(path string) error {
			if !filepath.IsAbs(path) {
				path = filepath.Join(m.dir, path)
			}
			start(path, false)
			return nil
		})
	}}}
	if os.Getenv("GALAXY_ANDROID") == "1" {
		options = append(options, ui.Action{Label: "Android에 백업", Run: func() { start(transferDir, true) }})
	}
	m.Menu("프로젝트 백업", options...)
}

func packBackup(source, target string) error {
	if inside(source, target) {
		return errors.New("백업 위치는 프로젝트 폴더 밖으로 선택하세요")
	}
	if info, err := os.Lstat(filepath.Join(source, ".git")); err == nil && !info.IsDir() {
		return errors.New("Git 기록이 프로젝트 폴더 밖에 있습니다")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(filepath.Join(source, ".git", "objects", "info", "alternates")); err == nil {
		return errors.New("Git 기록 일부가 다른 저장소에 있습니다")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.CreateTemp(filepath.Dir(target), ".galaxy-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(stage.Name())
	zipper := gzip.NewWriter(stage)
	archive := tar.NewWriter(zipper)
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == source {
			return err
		}
		name, err := filepath.Rel(source, path)
		if err != nil || !validBackupPath(name) {
			return fmt.Errorf("백업에 사용할 수 없는 경로입니다: %q", path)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		var link string
		switch {
		case info.IsDir(), info.Mode().IsRegular():
		case info.Mode()&os.ModeSymlink != 0:
			link, err = os.Readlink(path)
			if err != nil || !validBackupLink(name, link) {
				return fmt.Errorf("프로젝트 밖을 가리키는 링크입니다: %q", name)
			}
		default:
			return fmt.Errorf("백업할 수 없는 파일입니다: %q", name)
		}
		header, err := tar.FileInfoHeader(info, link)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(name)
		if err := archive.WriteHeader(header); err != nil || !info.Mode().IsRegular() {
			return err
		}
		file, err := os.OpenFile(path, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
		if err != nil {
			return err
		}
		defer file.Close()
		current, err := file.Stat()
		if err != nil || !current.Mode().IsRegular() {
			return fmt.Errorf("백업 도중 파일이 변경되었습니다: %q", name)
		}
		_, err = io.Copy(archive, file)
		return err
	})
	if err = errors.Join(err, archive.Close(), zipper.Close(), stage.Close()); err != nil {
		return err
	}
	return renameNew(stage.Name(), target)
}

func validBackupPath(name string) bool {
	return filepath.IsLocal(name) && name != "." && filepath.Clean(name) == name && !strings.ContainsAny(name, "\\\x00")
}

func validBackupLink(name, link string) bool {
	return link != "" && !filepath.IsAbs(link) && !strings.ContainsAny(link, "\\\x00") &&
		filepath.Clean(link) == link && filepath.IsLocal(filepath.Join(filepath.Dir(name), link))
}

func restoreBackup(source, dest string) error {
	stage, err := os.MkdirTemp(filepath.Dir(dest), ".galaxy-restore-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := extractBackup(source, stage); err != nil {
		return err
	}
	return renameNew(stage, dest)
}

func extractBackup(source, stage string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	zipper, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer zipper.Close()
	archive := tar.NewReader(zipper)
	links := make(map[string]string)
	for {
		entry, err := archive.Next()
		if err == io.EOF {
			if _, err := io.Copy(io.Discard, zipper); err != nil {
				return err
			}
			break
		}
		if err != nil {
			return err
		}
		name := strings.TrimSuffix(entry.Name, "/")
		if !validBackupPath(name) {
			return fmt.Errorf("백업에 올바르지 않은 경로가 있습니다: %q", name)
		}
		path := filepath.Join(stage, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return err
		}
		switch entry.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(path, 0700)
		case tar.TypeReg, tar.TypeRegA:
			err = writeNew(path, archive, 0600|entry.FileInfo().Mode().Perm()&0111)
		case tar.TypeSymlink:
			if _, exists := links[path]; exists || !validBackupLink(name, entry.Linkname) {
				return fmt.Errorf("백업의 링크가 올바르지 않습니다: %q", name)
			}
			links[path] = entry.Linkname
		default:
			return fmt.Errorf("백업에서 복원할 수 없는 항목입니다: %q", name)
		}
		if err != nil {
			return err
		}
	}
	// Install links last so archive entries always write through real directories.
	for path, link := range links {
		if err := os.Symlink(link, path); err != nil {
			return err
		}
	}
	return nil
}
