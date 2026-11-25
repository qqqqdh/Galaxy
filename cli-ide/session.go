package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type sessionTab struct {
	Path                 string
	Start, End           int
	ScrollRow, ScrollCol int
}

type editorSession struct {
	Active string
	Tabs   []sessionTab
}

func (i *ide) restoreSession(dir string) error {
	key := sha256.Sum256([]byte(i.root))
	i.sessionPath = filepath.Join(dir, fmt.Sprintf("%x.json", key))
	data, err := os.ReadFile(i.sessionPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var session editorSession
	if err := json.Unmarshal(data, &session); err != nil {
		return err
	}
	opened := map[string]*openDocument{}
	active := i.active
	for _, d := range i.documents {
		opened[d.disk.path] = d
	}
	var firstError error
	for _, tab := range session.Tabs {
		if !filepath.IsAbs(tab.Path) {
			continue
		}
		d := opened[tab.Path]
		if d == nil {
			disk, err := readDocument(tab.Path)
			if err != nil {
				if firstError == nil {
					firstError = err
				}
				continue
			}
			if !disk.exists {
				continue
			}
			d = opened[disk.path]
			if d == nil {
				i.addDocument(disk)
				d = i.active
				opened[disk.path] = d
			}
		}
		d.syntax.definitionSelection = &[2]int{tab.Start, tab.End}
		d.syntax.sessionOffset = &[2]int{max(0, tab.ScrollRow), max(0, tab.ScrollCol)}
	}
	if active == nil {
		active = opened[session.Active]
	}
	if active != nil {
		i.activate(active)
	}
	return firstError
}

func (i *ide) saveSession() error {
	if i.sessionPath == "" {
		return nil
	}
	session := editorSession{}
	if i.active != nil {
		session.Active = i.active.disk.path
	}
	for _, d := range i.documents {
		if d.disk.path == "" || !d.disk.exists {
			continue
		}
		_, start, end := d.editor.GetSelection()
		row, col := d.editor.GetOffset()
		if pending := d.syntax.definitionSelection; pending != nil {
			start, end = pending[0], pending[1]
		}
		if pending := d.syntax.sessionOffset; pending != nil {
			row, col = pending[0], pending[1]
		}
		session.Tabs = append(session.Tabs, sessionTab{d.disk.path, start, end, row, col})
	}
	dir := filepath.Dir(i.sessionPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := errors.Join(json.NewEncoder(f).Encode(session), f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), i.sessionPath)
}
