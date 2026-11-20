package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The first line is a complete checkpoint. Subsequent lines are edits to it.
type checkpoint struct {
	Version              int
	Path, Original, Text string
	Digest               [32]byte
	Mode                 fs.FileMode
	Exists, BOM, CRLF    bool
}
type draftEdit struct {
	From, To int
	Insert   []byte // Byte offsets can split two UTF-8 characters that share a prefix.
}
type draft struct {
	lock     *os.File
	path     string
	last     string
	logBytes int64
	end      int64
}

func newDraft(dir string) (*draft, error) {
	lock, err := os.CreateTemp(dir, "draft-*.lock")
	if err != nil {
		return nil, err
	}
	ok, err := lockDraft(lock)
	if !ok {
		lock.Close()
		os.Remove(lock.Name())
		if err == nil {
			err = errors.New("새 복구 파일이 사용 중입니다")
		}
		return nil, err
	}
	return &draft{lock: lock, path: strings.TrimSuffix(lock.Name(), ".lock") + ".jsonl"}, nil
}

func (r *draft) checkpoint(d *document, text string) error {
	f, err := os.CreateTemp(filepath.Dir(r.path), ".checkpoint-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	record := checkpoint{1, d.path, d.text, text, d.digest, d.mode, d.exists, d.bom, d.crlf}
	err = json.NewEncoder(f).Encode(record)
	n, posErr := f.Seek(0, io.SeekCurrent)
	closeErr := f.Close()
	if err = errors.Join(err, posErr, closeErr); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), r.path); err != nil {
		return err
	}
	r.last, r.logBytes, r.end = text, 0, n
	return nil
}

func difference(old, text string) draftEdit {
	a := 0
	for a < len(old) && a < len(text) && old[a] == text[a] {
		a++
	}
	b, c := len(old), len(text)
	for b > a && c > a && old[b-1] == text[c-1] {
		b--
		c--
	}
	return draftEdit{a, b, []byte(text[a:c])}
}

func (r *draft) write(d *document, text string) error {
	if r.end == 0 || r.logBytes > int64(max(64<<10, len(text))) {
		return r.checkpoint(d, text)
	}
	if r.last == text {
		return nil
	}
	data, err := json.Marshal(difference(r.last, text))
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(r.path, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	// Truncate a previous incomplete write before retrying.
	if err = f.Truncate(r.end); err == nil {
		_, err = f.WriteAt(data, r.end)
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		r.last = text
		r.logBytes += int64(len(data))
		r.end += int64(len(data))
	}
	return err
}

func (r *draft) read() (*document, string, error) {
	f, err := os.Open(r.path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, "", err
	}
	var c checkpoint
	if err = json.Unmarshal(line, &c); err != nil {
		return nil, "", err
	}
	if c.Version != 1 || (c.Path != "" && !filepath.IsAbs(c.Path)) {
		return nil, "", errors.New("잘못된 복구 파일 형식")
	}
	r.end = int64(len(line))
	text := []byte(c.Text)
	for {
		line, err = reader.ReadBytes('\n')
		if err == io.EOF {
			break
		} // An interrupted final edit has no newline.
		if err != nil {
			return nil, "", err
		}
		var edit draftEdit
		if err = json.Unmarshal(line, &edit); err != nil {
			return nil, "", err
		}
		if edit.From < 0 || edit.To < edit.From || edit.To > len(text) {
			return nil, "", errors.New("잘못된 복구 편집 범위")
		}
		tail := len(text) - edit.To
		size := edit.From + len(edit.Insert) + tail
		if size > len(text) {
			text = append(text, make([]byte, size-len(text))...)
		}
		copy(text[edit.From+len(edit.Insert):], text[edit.To:edit.To+tail])
		copy(text[edit.From:], edit.Insert)
		text = text[:size]
		r.end += int64(len(line))
		r.logBytes += int64(len(line))
	}
	r.last = string(text)
	return &document{path: c.Path, text: c.Original, digest: c.Digest, mode: c.Mode, exists: c.Exists, bom: c.BOM, crlf: c.CRLF}, r.last, nil
}

func (r *draft) discard() error {
	if err := os.Remove(r.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	r.lock.Close()
	return os.Remove(r.lock.Name())
}

func (i *ide) dropDraft(d *openDocument) bool {
	if d.draft != nil {
		if err := d.draft.discard(); err != nil {
			i.notify("복구 파일 정리 실패 · " + err.Error())
			return false
		}
		d.draft = nil
	}
	return true
}

func (i *ide) recordEdit(d *openDocument) {
	if i.draftDir == "" {
		return
	}
	text := d.editor.GetText()
	if text == d.disk.text {
		i.dropDraft(d)
		return
	}
	var err error
	if d.draft == nil {
		d.draft, err = newDraft(i.draftDir)
	}
	if err == nil {
		err = d.draft.write(d.disk, text)
	}
	if err != nil {
		i.notify("자동 복구 기록 실패 · " + err.Error())
	}
}

// Claim only abandoned drafts. A second running IDE keeps its own buffers.
func (i *ide) recoverDrafts(dir string) (int, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return 0, err
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	var failures []error
	count := 0
	for _, file := range files {
		if !file.Type().IsRegular() || !strings.HasPrefix(file.Name(), "draft-") || !strings.HasSuffix(file.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, file.Name())
		lock, err := os.OpenFile(strings.TrimSuffix(path, ".jsonl")+".lock", os.O_RDWR, 0600)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			failures = append(failures, err)
			continue
		}
		ok, err := lockDraft(lock)
		if !ok {
			lock.Close()
			if err != nil {
				failures = append(failures, err)
			}
			continue
		}
		r := &draft{lock: lock, path: path}
		d, text, err := r.read()
		if err != nil {
			lock.Close()
			if !os.IsNotExist(err) {
				failures = append(failures, fmt.Errorf("%s: %w", file.Name(), err))
			}
			continue
		}
		// A crash may happen after saving the source but before deleting the draft.
		if d.path != "" {
			current, err := readDocument(d.path)
			if err == nil && current.exists && current.text == text {
				if err = r.discard(); err != nil {
					failures = append(failures, err)
				}
				continue
			}
		}
		i.addDocument(d)
		i.active.editor.SetText(text, true)
		i.active.draft = r
		count++
	}
	i.draftDir = dir
	return count, errors.Join(failures...)
}
