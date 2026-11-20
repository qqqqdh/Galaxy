package main

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type searchFile struct {
	path, label, tab string
}

type projectFiles struct {
	root    string
	ignore  []string
	open    []searchFile
	buffers map[string]string
}

func (p projectFiles) excluded(name string) bool {
	return name == ".git" || name == "build" || strings.HasPrefix(name, "cmake-build-") || slices.Contains(p.ignore, name)
}

// Snapshot widgets on the UI thread. Workers only read the captured strings.
func (i *ide) projectFiles(withText bool) projectFiles {
	p := projectFiles{root: i.root, ignore: i.config.Ignore, buffers: map[string]string{}}
	for _, d := range i.documents {
		label := d.name() + " #" + d.id
		if d.disk.path != "" {
			rel, err := filepath.Rel(i.root, d.disk.path)
			if err != nil || !filepath.IsLocal(rel) || slices.ContainsFunc(strings.Split(filepath.ToSlash(rel), "/"), p.excluded) {
				continue
			}
			label = filepath.ToSlash(rel)
		}
		p.open = append(p.open, searchFile{d.disk.path, label, d.id})
		if withText {
			p.buffers[d.id] = d.editor.GetText()
		}
	}
	return p
}

func (p projectFiles) walk(ctx context.Context, visit func(searchFile) bool) error {
	seen := map[string]bool{}
	for _, f := range p.open {
		seen[f.path] = true
		if ctx.Err() != nil || !visit(f) {
			return nil
		}
	}
	return filepath.WalkDir(p.root, func(path string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return fs.SkipAll
		}
		if err != nil {
			if path == p.root {
				return err
			}
			return nil
		}
		if path == p.root {
			return nil
		}
		if p.excluded(entry.Name()) {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || seen[path] {
			return nil
		}
		rel, err := filepath.Rel(p.root, path)
		if err != nil {
			return err
		}
		if !visit(searchFile{path: path, label: filepath.ToSlash(rel)}) {
			return fs.SkipAll
		}
		return nil
	})
}

func (i *ide) openSearchFile(f searchFile) bool {
	for _, d := range i.documents {
		if d.id == f.tab || (f.path != "" && d.disk.path == f.path) {
			i.activate(d)
			return true
		}
	}
	if f.path != "" {
		d, err := readDocument(f.path)
		if err != nil {
			i.notify(err.Error())
			return false
		}
		if d.exists {
			i.addDocument(d)
			return true
		}
	}
	i.notify("문서가 닫혔거나 이동했습니다. 다시 검색하세요")
	return false
}

func (i *ide) quickOpen() {
	field := tview.NewInputField().SetLabel(" 파일명  ")
	list := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(field, 1, 0, true).AddItem(list, 0, 1, false)
	box.SetBorder(true).SetTitle(" 빠른 파일 열기 · ↑↓ 선택 · Enter 열기 · Esc 닫기 ")
	var files, matches []searchFile
	selectFile := func(index int) {
		if index >= 0 && index < len(matches) {
			f := matches[index]
			i.dismiss()
			i.openSearchFile(f)
		}
	}
	list.SetSelectedFunc(func(index int, _ string, _ string, _ rune) { selectFile(index) })
	filter := func(value string) {
		list.Clear()
		matches = nil
		q := strings.ToLower(value)
		for _, f := range files {
			if strings.Contains(strings.ToLower(f.label), q) {
				matches = append(matches, f)
				list.AddItem(tview.Escape(f.label), "", 0, nil)
				if len(matches) == 100 {
					break
				}
			}
		}
		if len(matches) == 0 {
			list.AddItem("일치하는 파일 없음", "", 0, nil)
		}
	}
	field.SetChangedFunc(filter).SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyEnter:
			selectFile(list.GetCurrentItem())
		case tcell.KeyUp:
			list.SetCurrentItem(max(0, list.GetCurrentItem()-1))
		case tcell.KeyDown:
			list.SetCurrentItem(min(list.GetItemCount()-1, list.GetCurrentItem()+1))
		default:
			return e
		}
		return nil
	})
	list.AddItem("파일 목록 읽는 중…", "", 0, nil)
	i.showDialog(box, 88, 18)
	i.pickerField = field
	ctx, cancel := context.WithCancel(context.Background())
	i.pickerCancel = cancel
	p := i.projectFiles(false)
	go func() {
		var found []searchFile
		err := p.walk(ctx, func(f searchFile) bool {
			if f.path != "" {
				found = append(found, f)
			}
			return true
		})
		if ctx.Err() != nil {
			return
		}
		i.app.QueueUpdateDraw(func() {
			if ctx.Err() != nil {
				return
			}
			files = found
			filter(field.GetText())
			if err != nil {
				list.AddItem(tview.Escape(err.Error()), "", 0, nil)
			}
		})
	}()
}

type searchHit struct {
	file         searchFile
	line, column int // One-based line and zero-based byte column in normalized editor text.
	preview      string
}

const maxSearchLines = 500

func (p projectFiles) search(ctx context.Context, q string) ([]searchHit, bool, error) {
	var hits []searchHit
	limited := false
	if q == "" {
		return nil, false, nil
	}
	err := p.walk(ctx, func(f searchFile) bool {
		text, open := p.buffers[f.tab]
		if !open {
			d, err := readDocument(f.path)
			if err != nil || !d.exists {
				return true
			}
			text = d.text
		}
		row := 0
		for line := range strings.SplitSeq(text, "\n") {
			if ctx.Err() != nil {
				return false
			}
			row++
			col := strings.Index(line, q)
			if col < 0 {
				continue
			}
			if len(hits) == maxSearchLines {
				limited = true
				return false
			}
			start := max(0, col-48)
			end := min(len(line), col+144)
			preview := strings.ToValidUTF8(line[start:end], "…")
			hits = append(hits, searchHit{f, row, col, strings.Clone(strings.TrimSpace(preview))})
		}
		return true
	})
	return hits, limited, err
}

func (i *ide) jumpToHit(hit searchHit, q string) {
	if !i.openSearchFile(hit.file) {
		return
	}
	offset, row := 0, 0
	for line := range strings.SplitSeq(i.active.editor.GetText(), "\n") {
		row++
		if row == hit.line {
			if hit.column <= len(line) && strings.HasPrefix(line[hit.column:], q) {
				start := offset + hit.column
				i.active.editor.Select(start, start+len(q))
				return
			}
			break
		}
		offset += len(line) + 1
	}
	i.notify("검색 후 내용이 바뀌었습니다. 다시 검색하세요")
}

func (i *ide) makeSearchPane() *tview.Flex {
	i.searchField = tview.NewInputField().SetLabel(" 찾을 내용  ")
	i.searchList = tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	i.searchList.AddItem("검색어 입력 후 Enter", "", 0, nil)
	i.searchField.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyEnter:
			i.startSearch()
		case tcell.KeyDown, tcell.KeyTab:
			i.app.SetFocus(i.searchList)
		default:
			return e
		}
		return nil
	})
	i.searchList.SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		if e.Key() == tcell.KeyTab || (e.Key() == tcell.KeyUp && i.searchList.GetCurrentItem() == 0) {
			i.app.SetFocus(i.searchField)
			return nil
		}
		return e
	})
	return tview.NewFlex().SetDirection(tview.FlexRow).AddItem(i.searchField, 1, 0, true).AddItem(i.searchList, 0, 1, false)
}

func (i *ide) startSearch() {
	if i.searchCancel != nil {
		i.searchCancel()
	}
	list := i.searchList.Clear()
	q := i.searchField.GetText()
	if q == "" {
		list.AddItem("검색어 입력 후 Enter", "", 0, nil)
		return
	}
	list.AddItem("검색 중…", "", 0, nil)
	ctx, cancel := context.WithCancel(context.Background())
	i.searchCancel = cancel
	p := i.projectFiles(true)
	go func() {
		hits, limited, err := p.search(ctx, q)
		if ctx.Err() != nil {
			return
		}
		i.app.QueueUpdateDraw(func() {
			if ctx.Err() != nil {
				return
			}
			list.Clear()
			for _, h := range hits {
				list.AddItem(tview.Escape(fmt.Sprintf("%s:%d  %s", h.file.label, h.line, h.preview)), "", 0, func() { i.jumpToHit(h, q) })
			}
			if err != nil {
				list.AddItem(tview.Escape(err.Error()), "", 0, nil)
			} else if len(hits) == 0 {
				list.AddItem("일치하는 줄 없음", "", 0, nil)
			}
			if limited {
				list.AddItem("처음 500줄만 표시 · 검색어를 좁혀 주세요", "", 0, nil)
			}
		})
	}()
}
