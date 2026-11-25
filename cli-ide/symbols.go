package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (i *ide) showReferences(data json.RawMessage) {
	var locations []location
	if err := json.Unmarshal(data, &locations); err != nil {
		i.notify(err.Error())
		return
	}
	list := tview.NewList().ShowSecondaryText(false).SetHighlightFullLine(true)
	seen := map[location]bool{}
	for _, ref := range locations {
		path, err := uriPath(ref.URI)
		if err != nil || ref.Range.Start.Line < 0 || seen[ref] {
			continue
		}
		seen[ref] = true
		label := path
		if rel, err := filepath.Rel(i.root, path); err == nil && filepath.IsLocal(rel) {
			label = rel
		}
		pos := ref.Range.Start
		list.AddItem(tview.Escape(fmt.Sprintf("%s:%d:%d", label, pos.Line+1, pos.Character+1)), "", 0, func() {
			i.dismiss()
			if _, err := os.Stat(path); err != nil {
				i.notify(err.Error())
				return
			}
			i.jumpLocation(ref, "참조")
		})
	}
	if list.GetItemCount() == 0 {
		i.notify("참조를 찾지 못했습니다")
		return
	}
	list.SetBorder(true).SetTitle(fmt.Sprintf(" 참조 %d개 · Enter로 이동 ", list.GetItemCount()))
	i.showDialog(list, 88, min(24, list.GetItemCount()+2))
}

func (i *ide) renameSymbol() {
	d := i.active
	if toolsFor(d.disk.path).id == "" {
		i.notify("언어 서버를 지원하는 소스 파일을 여세요")
		return
	}
	i.prompt("심볼 이름 변경", "새 이름", "", func(name string) {
		i.askLanguageServer("rename", false, name)
	})
}

type lspTextEdit struct {
	Range   *lspRange `json:"range"`
	NewText string    `json:"newText"`
}

func decodeRename(data json.RawMessage, versionMatches func(string, int) bool) (map[string][]lspTextEdit, error) {
	var edit struct {
		Changes         map[string][]lspTextEdit
		DocumentChanges []struct {
			Kind         string
			TextDocument struct {
				URI     string
				Version *int
			}
			Edits []lspTextEdit
		}
	}
	if err := json.Unmarshal(data, &edit); err != nil {
		return nil, err
	}
	if len(edit.DocumentChanges) == 0 {
		return edit.Changes, nil
	}
	files := make(map[string][]lspTextEdit, len(edit.DocumentChanges))
	for _, change := range edit.DocumentChanges {
		doc := change.TextDocument
		if change.Kind != "" || doc.URI == "" {
			return nil, errors.New("파일 생성·삭제·이동이 필요한 이름 변경은 편집기에서 적용할 수 없습니다")
		}
		if doc.Version != nil {
			path, err := uriPath(doc.URI)
			if err != nil || !versionMatches(path, *doc.Version) {
				return nil, errors.New("언어 서버의 문서 버전이 바뀌었습니다. 이름 변경을 다시 요청하세요")
			}
		}
		files[doc.URI] = append(files[doc.URI], change.Edits...)
	}
	return files, nil
}

type renameLocation struct {
	start, end, line int
	text             string
}

type renameFile struct {
	disk          *document
	before, after string
	changes       []renameLocation
}

func planRename(root string, data json.RawMessage, snapshots map[string]string) ([]renameFile, error) {
	var edits map[string][]lspTextEdit
	if err := json.Unmarshal(data, &edits); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var plans []renameFile
	for uri, changes := range edits {
		if len(changes) == 0 {
			continue
		}
		path, err := uriPath(uri)
		if err != nil {
			return nil, err
		}
		path, err = filepath.EvalSymlinks(path)
		if err != nil {
			return nil, err
		}
		if rel, err := filepath.Rel(root, path); err != nil || !filepath.IsLocal(rel) || seen[path] {
			return nil, errors.New("프로젝트 밖의 파일이나 중복 경로가 포함되어 있습니다")
		}
		seen[path] = true
		disk, err := readDocument(path)
		if err != nil {
			return nil, err
		}
		if !disk.exists || disk.mode&0222 == 0 {
			return nil, fmt.Errorf("편집할 수 없는 파일 · %s", path)
		}
		before, open := snapshots[path]
		if !open {
			before = disk.text
		}
		plan := renameFile{disk: disk, before: before}
		for _, edit := range changes {
			if edit.Range == nil || !utf8.ValidString(edit.NewText) || strings.ContainsRune(edit.NewText, 0) {
				return nil, fmt.Errorf("유효하지 않은 이름 변경 · %s", path)
			}
			start, validStart := bytePosition(before, edit.Range.Start)
			end, validEnd := bytePosition(before, edit.Range.End)
			if !validStart || !validEnd || start > end {
				return nil, fmt.Errorf("위치가 맞지 않는 이름 변경 · %s", path)
			}
			value := strings.ReplaceAll(edit.NewText, "\r\n", "\n")
			if before[start:end] != value {
				plan.changes = append(plan.changes, renameLocation{start, end, edit.Range.Start.Line + 1, value})
			}
		}
		slices.SortStableFunc(plan.changes, func(a, b renameLocation) int {
			return cmp.Or(cmp.Compare(a.start, b.start), cmp.Compare(a.end, b.end))
		})
		var after strings.Builder
		offset := 0
		for _, change := range plan.changes {
			if change.start < offset {
				return nil, fmt.Errorf("서로 겹치는 이름 변경 · %s", path)
			}
			after.WriteString(before[offset:change.start])
			after.WriteString(change.text)
			offset = change.end
		}
		after.WriteString(before[offset:])
		if after.Len() > maxFileSize {
			return nil, fmt.Errorf("편집 가능한 파일 크기를 초과합니다 · %s", path)
		}
		if len(plan.changes) > 0 {
			plan.after = after.String()
			plans = append(plans, plan)
		}
	}
	slices.SortFunc(plans, func(a, b renameFile) int { return cmp.Compare(a.disk.path, b.disk.path) })
	return plans, nil
}

func (i *ide) showRename(plans []renameFile) {
	if len(plans) == 0 {
		i.notify("변경할 심볼이 없습니다")
		return
	}
	var preview strings.Builder
	for _, plan := range plans {
		path, _ := filepath.Rel(i.root, plan.disk.path)
		fmt.Fprintln(&preview, path)
		for _, change := range plan.changes {
			fmt.Fprintf(&preview, "  %d행  %q → %q\n", change.line, plan.before[change.start:change.end], change.text)
		}
		fmt.Fprintln(&preview)
	}
	text := tview.NewTextView().SetScrollable(true).SetWordWrap(true).SetText(preview.String())
	form := tview.NewForm().SetButtonsAlign(tview.AlignCenter)
	form.AddButton("적용", func() { i.dismiss(); i.applyRename(plans) }).AddButton("취소", i.dismiss)
	text.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyTab {
			i.app.SetFocus(form)
			return nil
		}
		return event
	})
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(text, 0, 1, true).AddItem(form, 3, 0, false)
	box.SetBorder(true).SetTitle(fmt.Sprintf(" 이름 변경 미리보기 · 파일 %d개 · Tab으로 적용 버튼 이동 ", len(plans)))
	i.showDialog(box, 94, 24)
}

func (i *ide) applyRename(plans []renameFile) {
	opened := make(map[string]*openDocument, len(i.documents))
	for _, d := range i.documents {
		opened[d.disk.path] = d
	}
	for _, plan := range plans {
		fresh, err := readDocument(plan.disk.path)
		d := opened[plan.disk.path]
		if err != nil || !fresh.exists || fresh.path != plan.disk.path || fresh.digest != plan.disk.digest ||
			fresh.mode&0222 == 0 || d != nil && (d.editor.GetText() != plan.before || d.disk.digest != fresh.digest) ||
			d == nil && fresh.text != plan.before {
			i.notify("미리보기 이후 파일이나 편집 내용이 바뀌었습니다 · " + filepath.Base(plan.disk.path))
			return
		}
	}
	active := i.active
	// Validate every change first. Each file gets a single undoable edit in its buffer.
	for _, plan := range plans {
		d := opened[plan.disk.path]
		if d == nil {
			i.addDocument(plan.disk)
			d = i.active
		}
		_, offset, _ := d.editor.GetSelection()
		d.editor.Replace(0, len(plan.before), plan.after)
		offset = min(offset, len(plan.after))
		d.editor.Select(offset, offset)
		d.syntax.definitionSelection = &[2]int{offset, offset}
	}
	if active != nil {
		i.activate(active)
	}
	i.notify(fmt.Sprintf("파일 %d개에 적용했습니다 · 변경된 탭을 저장하세요", len(plans)))
}
