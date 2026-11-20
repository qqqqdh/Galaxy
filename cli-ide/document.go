package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxFileSize = 8 << 20

var errConflict = errors.New("디스크의 파일이 바뀌었습니다. 내용을 확인한 뒤 덮어쓰기를 선택하세요")

type document struct {
	path   string
	text   string
	digest [32]byte
	exists bool
	bom    bool
	crlf   bool
	mode   fs.FileMode
}

func readDocument(path string) (*document, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		path = resolved
	} else if !os.IsNotExist(err) {
		return nil, err
	} else if info, statErr := os.Lstat(path); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("연결 대상이 없는 심볼릭 링크입니다: %s", path)
	}
	d := &document{path: path, mode: 0644}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return d, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("일반 텍스트 파일을 선택하세요: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxFileSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxFileSize {
		return nil, errors.New("8 MiB 이하의 텍스트 파일을 열 수 있습니다")
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, errors.New("UTF-8 텍스트 파일을 선택하세요. 바이너리와 다른 인코딩은 지원하지 않습니다")
	}
	d.exists, d.mode, d.digest = true, info.Mode().Perm(), sha256.Sum256(data)
	d.bom = bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf})
	d.crlf = bytes.Contains(data, []byte("\r\n"))
	d.text = strings.ReplaceAll(strings.TrimPrefix(string(data), "\ufeff"), "\r\n", "\n")
	return d, nil
}

func saveDocument(d *document, path, text string, overwrite bool) error {
	target, err := readDocument(path)
	if err != nil {
		return err
	}
	if !overwrite && target.exists && (target.path != d.path || !d.exists || target.digest != d.digest) {
		return errConflict
	}
	if !overwrite && !target.exists && d.exists && target.path == d.path {
		return errConflict
	}
	if target.exists && target.mode&0222 == 0 {
		return os.ErrPermission
	}
	data := text
	if d.crlf {
		data = strings.ReplaceAll(data, "\n", "\r\n")
	}
	if d.bom {
		data = "\ufeff" + data
	}
	if len(data) > maxFileSize {
		return errors.New("저장할 내용은 8 MiB 이하여야 합니다")
	}
	if !utf8.ValidString(data) || strings.IndexByte(data, 0) >= 0 {
		return errors.New("UTF-8 텍스트만 저장할 수 있습니다")
	}
	if err := os.MkdirAll(filepath.Dir(target.path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(target.path), ".galaxy-save-*")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	defer f.Close()
	mode := d.mode
	if target.exists {
		mode = target.mode
	}
	if err := f.Chmod(mode); err != nil {
		return err
	}
	if _, err := f.WriteString(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	latest, err := readDocument(target.path)
	if err != nil {
		return err
	}
	if latest.exists != target.exists || latest.digest != target.digest {
		return errConflict
	}
	if err := os.Rename(temporary, target.path); err != nil {
		return err
	}
	d.path, d.text, d.exists = target.path, text, true
	d.mode, d.digest = mode, sha256.Sum256([]byte(data))
	return nil
}
