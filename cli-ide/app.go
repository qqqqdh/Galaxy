package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	background = tcell.GetColor("#111722")
	panel      = tcell.GetColor("#192332")
	foreground = tcell.GetColor("#dde6ef")
	muted      = tcell.GetColor("#899bae")
	accent     = tcell.GetColor("#8edbc6")
)

type openDocument struct {
	draft  *draft
	disk   *document
	editor *tview.TextArea
	syntax *syntaxView
	button *tview.Button
	id     string
}

func (d *openDocument) dirty() bool {
	return d.editor.GetTextLength() != len(d.disk.text) || d.editor.GetText() != d.disk.text
}

func (d *openDocument) name() string {
	if d.disk.path == "" {
		return "새 파일"
	}
	return filepath.Base(d.disk.path)
}

type ide struct {
	languages                  languageClient
	languageLife               context.Context
	stopLanguages, cancelHint  context.CancelFunc
	hintTimer                  *time.Timer
	hints                      *tview.List
	hintOffset                 int
	acceptHint                 func(int)
	changeList                 *tview.List
	changesCancel, diffCancel  context.CancelFunc
	searchField, pickerField   *tview.InputField
	searchList                 *tview.List
	searchCancel, pickerCancel context.CancelFunc
	draftDir                   string
	live, dockOpen             bool
	fullTerminal               bool
	timer                      *time.Timer
	cancel                     context.CancelFunc
	dock                       *tview.Pages
	shell                      *shellPane
	problemList                *tview.List
	problemButton              *tview.Button
	app                        *tview.Application
	root                       string
	config                     config
	pages                      *tview.Pages
	editors                    *tview.Pages
	body                       *tview.Flex
	tabs                       *tview.Flex
	tree                       *tview.TreeView
	status                     *tview.TextView
	documents                  []*openDocument
	active                     *openDocument
	clipboard                  string
	query                      string
	message                    string
	dialog                     bool
	dialogGrid                 *tview.Grid
	dialogWidth, dialogHeight  int
	showTree                   bool
	treeShown                  bool
	nextID                     int
}

func newIDE(root string, config config) *ide {
	// Match readDocument's canonical paths when a project is opened through a symlink.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	tview.Styles.PrimitiveBackgroundColor = background
	tview.Styles.ContrastBackgroundColor = panel
	tview.Styles.MoreContrastBackgroundColor = tcell.GetColor("#31465b")
	tview.Styles.PrimaryTextColor = foreground
	tview.Styles.SecondaryTextColor = accent
	tview.Styles.BorderColor = tcell.GetColor("#314054")
	tview.TabSize = config.IndentSize
	i := &ide{app: tview.NewApplication(), root: root, config: config, showTree: true}
	i.languageLife, i.stopLanguages = context.WithCancel(context.Background())
	i.languages.onReport = func(report languageReport) {
		if toolsFor(report.path).server == "clangd" || i.languageLife.Err() != nil {
			return
		}
		go i.app.QueueUpdateDraw(func() {
			if i.active != nil && i.active.disk.path == report.path && i.active.editor.GetText() == report.text {
				i.showProblems(report.problems(), "문제 없음")
			}
		})
	}
	i.pages, i.editors = tview.NewPages(), tview.NewPages()
	i.tabs, i.body = tview.NewFlex(), tview.NewFlex()
	i.status = tview.NewTextView().SetDynamicColors(true)
	i.status.SetBackgroundColor(panel)
	i.tree = tview.NewTreeView().SetGraphicsColor(muted)
	i.tree.SetBorder(true).SetTitle(" 파일 ").SetTitleColor(muted)
	i.tree.SetSelectedFunc(func(node *tview.TreeNode) {
		path, ok := node.GetReference().(string)
		if !ok {
			return
		}
		info, err := os.Stat(path)
		if err != nil {
			i.notify(err.Error())
			return
		}
		if info.IsDir() {
			if !node.IsExpanded() {
				i.populate(node, path)
			}
			node.SetExpanded(!node.IsExpanded())
		} else if err := i.openFile(path); err != nil {
			i.notify(err.Error())
		}
	})
	i.refreshTree()
	name := tview.NewTextView().SetDynamicColors(true).SetText(" [#8edbc6::b]GALAXY IDE[-::-]  " + tview.Escape(filepath.Base(root)))
	name.SetBackgroundColor(panel)
	header := tview.NewFlex().AddItem(name, 0, 1, false)
	for _, item := range []struct {
		label string
		run   func()
	}{{"파일", i.fileMenu}, {"실행", i.taskMenu}, {"AI", i.assistantMenu}, {"도움말", i.help}} {
		button := tview.NewButton(item.label).SetSelectedFunc(item.run)
		button.SetStyle(tcell.StyleDefault.Background(panel).Foreground(foreground))
		header.AddItem(button, 10, 0, false)
	}
	editing := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(i.tabs, 1, 0, false).AddItem(i.editors, 0, 1, true)
	i.body.AddItem(editing, 0, 1, true)
	footer := tview.NewTextView().SetText(" Ctrl+S 저장  Ctrl+O 열기  Ctrl+F 찾기  Ctrl+R 실행  Ctrl+E 메뉴  Ctrl+Q 종료")
	footer.SetBackgroundColor(panel)
	footer.SetTextColor(muted)
	bar := i.makeDock()
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).AddItem(i.body, 0, 1, true).
		AddItem(bar, 1, 0, false).AddItem(i.dock, 6, 0, false).AddItem(i.status, 1, 0, false).AddItem(footer, 1, 0, false)
	i.pages.AddPage("main", layout, true, true)
	fullBar := tview.NewFlex().AddItem(tview.NewButton("편집기로").SetSelectedFunc(i.toggleFullTerminal), 12, 0, false).
		AddItem(tview.NewTextView().SetText(" 프로젝트 터미널 · Ctrl+T 돌아가기"), 0, 1, false)
	i.pages.AddPage("terminal-full", tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(fullBar, 1, 0, false).AddItem(i.shell, 0, 1, true), true, false)
	i.app.SetRoot(i.pages, true).EnableMouse(true).EnablePaste(true)
	i.app.SetInputCapture(i.handleKey)
	i.app.SetAfterDrawFunc(i.drawHints)
	i.app.SetMouseCapture(i.hintMouse)
	var editorTitle string
	i.app.SetBeforeDrawFunc(func(screen tcell.Screen) bool {
		if i.acceptHint != nil && i.active != nil {
			_, a, b := i.active.editor.GetSelection()
			if !i.active.editor.HasFocus() || a != i.hintOffset || b != a {
				i.clearHints()
			}
		}
		title := "Galaxy IDE"
		if i.shell.HasFocus() {
			title = "Galaxy IDE · 터미널"
		} else if (i.pickerField != nil && i.pickerField.HasFocus()) || i.searchField.HasFocus() ||
			(!i.dialog && i.active != nil && i.active.editor.HasFocus() && !i.active.editor.HasSelection()) {
			title = "Galaxy IDE · 편집"
		}
		if title != editorTitle {
			screen.SetTitle(title)
			editorTitle = title
		}
		width, height := screen.Size()
		dockHeight := 0
		if i.dockOpen {
			dockHeight = max(1, min(10, height/4))
		}
		layout.ResizeItem(i.dock, dockHeight, 0)
		if i.dialog {
			i.dialogGrid.SetRows(0, min(i.dialogHeight, max(1, height-2)), 0).
				SetColumns(0, min(i.dialogWidth, max(1, width-2)), 0)
		}
		visible := i.showTree && width >= 80
		if visible != i.treeShown {
			i.body.RemoveItem(i.tree)
			if visible {
				i.body.AddItem(i.tree, 26, 0, false)
				i.body.RemoveItem(editing)
				i.body.AddItem(editing, 0, 1, true)
			} else if i.tree.HasFocus() && i.active != nil {
				go i.app.QueueUpdateDraw(func() {
					if !i.dialog && !i.treeShown {
						i.app.SetFocus(i.active.editor)
					}
				})
			}
			i.treeShown = visible
		}
		i.updateStatus()
		return false
	})
	return i
}

func (i *ide) newFile() { i.addDocument(&document{mode: 0644}) }

func (i *ide) addDocument(disk *document) {
	i.nextID++
	d := &openDocument{disk: disk, id: strconv.Itoa(i.nextID)}
	d.editor = tview.NewTextArea().SetWrap(false).SetText(disk.text, false).
		SetPlaceholder("여기에 바로 입력하세요. Ctrl+S로 저장합니다.").
		SetTextStyle(tcell.StyleDefault.Foreground(foreground).Background(background)).
		SetSelectedStyle(tcell.StyleDefault.Foreground(tcell.ColorWhite).Background(tcell.GetColor("#345c76")))
	d.editor.SetBorder(true).SetTitle(" " + tview.Escape(d.name()) + " ").SetTitleColor(muted)
	d.editor.SetClipboard(func(text string) { i.clipboard = text }, func() string { return i.clipboard })
	d.syntax = &syntaxView{TextArea: d.editor, app: i.app}
	d.syntax.setFilename(disk.path)
	d.editor.SetChangedFunc(func() {
		d.syntax.invalidate()
		if i.active == d {
			i.checkActive()
		}
		i.message = ""
		i.updateTab(d)
		i.recordEdit(d)
		if i.live && i.active == d {
			i.scheduleCompletion(d)
		}
	})
	d.editor.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			_, start, end := d.editor.GetSelection()
			before := d.editor.GetText()[:start]
			line := before[strings.LastIndex(before, "\n")+1:]
			indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
			d.editor.Replace(start, end, "\n"+indent)
			return nil
		}
		if event.Key() == tcell.KeyTab && !i.config.UseTabs {
			_, start, end := d.editor.GetSelection()
			_, _, _, column := d.editor.GetCursor()
			d.editor.Replace(start, end, strings.Repeat(" ", i.config.IndentSize-column%i.config.IndentSize))
			return nil
		}
		return event
	})
	d.button = tview.NewButton(d.name()).SetSelectedFunc(func() { i.activate(d) })
	i.documents = append(i.documents, d)
	i.editors.AddPage(d.id, d.syntax, true, false)
	i.tabs.AddItem(d.button, 0, 1, false)
	i.activate(d)
}

func (i *ide) updateTab(d *openDocument) {
	label := d.name()
	if d.dirty() {
		label += " *"
	}
	d.button.SetLabel(label)
	d.editor.SetTitle(" " + tview.Escape(label) + " ")
}

func (i *ide) activate(d *openDocument) {
	i.clearHints()
	i.active = d
	i.checkActive()
	for _, item := range i.documents {
		color := muted
		if item == d {
			color = accent
		}
		item.button.SetStyle(tcell.StyleDefault.Background(panel).Foreground(color))
	}
	i.editors.SwitchToPage(d.id)
	i.app.SetFocus(d.editor)
	i.message = ""
}

func (i *ide) resolve(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(i.root, path)
	}
	return filepath.Clean(path)
}

func (i *ide) openFile(path string) error {
	disk, err := readDocument(i.resolve(path))
	if err != nil {
		return err
	}
	for _, d := range i.documents {
		if d.disk.path == disk.path {
			i.activate(d)
			return nil
		}
	}
	i.addDocument(disk)
	return nil
}

func (i *ide) updateStatus() {
	if i.active == nil {
		return
	}
	d := i.active
	_, _, row, col := d.editor.GetCursor()
	path := d.disk.path
	if path == "" {
		path = "아직 저장하지 않은 파일"
	} else if relative, err := filepath.Rel(i.root, path); err == nil {
		path = relative
	}
	state := "저장됨"
	if d.dirty() || !d.disk.exists {
		state = "미저장"
	}
	if i.message != "" {
		i.status.SetText(" " + tview.Escape(i.message))
		return
	}
	eol := "LF"
	if d.disk.crlf {
		eol = "CRLF"
	}
	i.status.SetText(fmt.Sprintf(" %s  ·  %s  ·  %d행 %d열  ·  UTF-8 / %s", tview.Escape(path), state, row+1, col+1, eol))
}

func (i *ide) notify(message string) { i.message = message }

func (i *ide) handleKey(event *tcell.EventKey) *tcell.EventKey {
	if i.dialog {
		if event.Key() == tcell.KeyEscape || event.Key() == tcell.KeyCtrlC {
			i.dismiss()
			return nil
		}
		return event
	}
	if event.Key() == tcell.KeyF8 || i.fullTerminal && event.Key() == tcell.KeyCtrlT {
		i.toggleFullTerminal()
		return nil
	}
	if event.Key() == tcell.KeyCtrlT {
		if i.shell.HasFocus() {
			i.app.SetFocus(i.active.editor)
		} else {
			i.openDock("터미널")
		}
		return nil
	}
	if i.shell.HasFocus() {
		if event.Key() == tcell.KeyCtrlC {
			return tcell.NewEventKey(tcell.KeyCtrlC, 0, tcell.ModNone)
		}
		return event
	}
	if i.active != nil && i.active.editor.HasFocus() && toolsFor(i.active.disk.path).id != "" {
		switch event.Key() {
		case tcell.KeyF12, tcell.KeyCtrlRightSq:
			i.askLanguageServer("definition", false)
			return nil
		case tcell.KeyCtrlSpace, tcell.KeyCtrlK:
			i.askLanguageServer("completion", false)
			return nil
		}
		if i.acceptHint != nil {
			switch event.Key() {
			case tcell.KeyTab:
				i.acceptHint(i.hints.GetCurrentItem())
				return nil
			case tcell.KeyDown:
				i.hints.SetCurrentItem(min(i.hints.GetItemCount()-1, i.hints.GetCurrentItem()+1))
				return nil
			case tcell.KeyUp:
				i.hints.SetCurrentItem(max(0, i.hints.GetCurrentItem()-1))
				return nil
			case tcell.KeyEscape:
				i.clearHints()
				return nil
			}
			i.clearHints()
		}
	}
	if event.Key() == tcell.KeyCtrlJ {
		i.openDock("문제")
		return nil
	}
	if event.Key() == tcell.KeyCtrlP {
		i.quickOpen()
		return nil
	}
	if i.searchField.HasFocus() || i.searchList.HasFocus() {
		if event.Key() == tcell.KeyEscape {
			i.app.SetFocus(i.active.editor)
			return nil
		}
		return event
	}
	if event.Key() == tcell.KeyEscape && (i.problemList.HasFocus() || i.changeList.HasFocus()) {
		i.app.SetFocus(i.active.editor)
		return nil
	}
	switch event.Key() {
	case tcell.KeyCtrlN:
		i.newFile()
	case tcell.KeyCtrlO:
		i.openPrompt()
	case tcell.KeyCtrlS:
		i.save(i.active, "", nil)
	case tcell.KeyCtrlW:
		i.closeFile()
	case tcell.KeyCtrlQ:
		i.quit(0)
	case tcell.KeyCtrlC:
		if i.app.GetFocus() == i.active.editor {
			return tcell.NewEventKey(tcell.KeyCtrlQ, 0, tcell.ModNone)
		}
	case tcell.KeyCtrlA:
		if i.app.GetFocus() == i.active.editor {
			return tcell.NewEventKey(tcell.KeyCtrlL, 0, tcell.ModNone)
		}
	case tcell.KeyCtrlF:
		i.findPrompt(false)
	case tcell.KeyCtrlH:
		i.findPrompt(true)
	case tcell.KeyCtrlG:
		i.prompt("행으로 이동", "행 번호", "", func(value string) {
			line, err := strconv.Atoi(value)
			if err != nil || line < 1 {
				i.notify("1 이상의 행 번호를 입력하세요")
				return
			}
			text, offset := i.active.editor.GetText(), 0
			for n := 1; n < line; n++ {
				next := strings.IndexByte(text[offset:], '\n')
				if next < 0 {
					offset = len(text)
					break
				}
				offset += next + 1
			}
			i.active.editor.Select(offset, offset)
		})
	case tcell.KeyF1:
		i.help()
	case tcell.KeyF2:
		i.showTree = !i.showTree
	case tcell.KeyF3:
		i.findNext()
	case tcell.KeyF4:
		for index, d := range i.documents {
			if d == i.active {
				i.activate(i.documents[(index+1)%len(i.documents)])
				break
			}
		}
	case tcell.KeyF5, tcell.KeyCtrlR:
		if event.Key() == tcell.KeyCtrlR {
			if build, ok := i.defaultBuild(); ok {
				i.runTask(build)
				return nil
			}
		}
		i.taskMenu()
	case tcell.KeyF6, tcell.KeyCtrlB:
		if i.app.GetFocus() == i.tree {
			i.app.SetFocus(i.active.editor)
		} else if i.treeShown {
			i.app.SetFocus(i.tree)
		}
	case tcell.KeyF7:
		i.runTask(shellTask())
	case tcell.KeyF9, tcell.KeyCtrlE:
		i.fileMenu()
	default:
		return event
	}
	return nil
}

func (i *ide) refreshTree() {
	root := tview.NewTreeNode(tview.Escape(filepath.Base(i.root))).SetReference(i.root).SetColor(accent).SetExpanded(true)
	i.populate(root, i.root)
	i.tree.SetRoot(root).SetCurrentNode(root)
}

func (i *ide) populate(node *tview.TreeNode, path string) {
	entries, err := os.ReadDir(path)
	if err != nil {
		i.notify(err.Error())
		return
	}
	node.ClearChildren()
	sort.SliceStable(entries, func(a, b int) bool {
		if entries[a].IsDir() != entries[b].IsDir() {
			return entries[a].IsDir()
		}
		return strings.ToLower(entries[a].Name()) < strings.ToLower(entries[b].Name())
	})
	for _, entry := range entries {
		ignored := false
		for _, name := range i.config.Ignore {
			if entry.Name() == name {
				ignored = true
				break
			}
		}
		if ignored {
			continue
		}
		label := entry.Name()
		color := foreground
		if entry.IsDir() {
			label += "/"
			color = accent
		}
		child := tview.NewTreeNode(tview.Escape(label)).SetReference(filepath.Join(path, entry.Name())).SetColor(color).SetExpanded(false)
		node.AddChild(child)
	}
}

func (i *ide) save(d *openDocument, path string, after func()) {
	if path == "" {
		path = d.disk.path
	}
	if path == "" {
		i.prompt("새 파일 저장", "파일 경로", "", func(value string) { i.save(d, i.resolve(value), after) })
		return
	}
	for _, other := range i.documents {
		if other != d && other.disk.path == i.resolve(path) {
			i.notify("다른 탭에서 열린 파일입니다. 그 탭에서 저장하세요")
			return
		}
	}
	err := saveDocument(d.disk, path, d.editor.GetText(), false)
	if errors.Is(err, errConflict) {
		i.choose("파일 덮어쓰기", path+"\n\n외부 변경 또는 기존 파일이 있습니다. 현재 편집 내용으로 덮어쓸까요?", []string{"덮어쓰기", "취소"}, func(index int) {
			if index == 0 {
				if err := saveDocument(d.disk, path, d.editor.GetText(), true); err != nil {
					i.notify(err.Error())
					return
				}
				i.saved(d, after)
			}
		})
		return
	}
	if err != nil {
		i.notify(err.Error())
		return
	}
	i.saved(d, after)
}

func (i *ide) saved(d *openDocument, after func()) {
	d.syntax.setFilename(d.disk.path)
	if i.active == d {
		i.checkActive()
	}
	i.updateTab(d)
	i.refreshTree()
	i.notify("저장했습니다 · " + d.disk.path)
	if !i.dropDraft(d) {
		return
	}
	if d.disk.path == filepath.Join(i.root, ".galaxy-ide.json") {
		i.reloadConfig()
	}
	if after != nil {
		after()
	}
}

func (i *ide) guard(d *openDocument, after func()) {
	if !d.dirty() {
		after()
		return
	}
	i.choose("저장하지 않은 변경", d.name()+"의 변경 내용을 저장할까요?", []string{"저장", "버리기", "취소"}, func(index int) {
		if index == 0 {
			i.save(d, "", after)
		} else if index == 1 {
			after()
		}
	})
}

func (i *ide) closeFile() {
	d := i.active
	i.guard(d, func() {
		if !i.dropDraft(d) {
			return
		}
		for n, candidate := range i.documents {
			if candidate == d {
				i.documents = append(i.documents[:n], i.documents[n+1:]...)
				break
			}
		}
		i.editors.RemovePage(d.id)
		i.tabs.RemoveItem(d.button)
		if len(i.documents) == 0 {
			i.newFile()
		} else {
			i.activate(i.documents[len(i.documents)-1])
		}
	})
}

func (i *ide) quit(index int) {
	if index >= len(i.documents) {
		for _, d := range i.documents {
			if !i.dropDraft(d) {
				return
			}
		}
		i.cancelCheck()
		i.clearHints()
		i.stopLanguages()
		i.languages.close()
		if i.changesCancel != nil {
			i.changesCancel()
		}
		if i.searchCancel != nil {
			i.searchCancel()
		}
		i.shell.close()
		i.app.Stop()
		return
	}
	i.guard(i.documents[index], func() { i.quit(index + 1) })
}
