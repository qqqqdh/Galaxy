package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type modalLayer struct{ *tview.Grid }

func (m modalLayer) MouseHandler() func(tview.MouseAction, *tcell.EventMouse, func(tview.Primitive)) (bool, tview.Primitive) {
	handle := m.Grid.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, focus func(tview.Primitive)) (bool, tview.Primitive) {
		// Lists act on click; the editor underneath must not capture the preceding press.
		_, capture := handle(action, event, focus)
		return true, capture
	}
}

func (i *ide) showDialog(content tview.Primitive, width, height int) {
	i.clearHints()
	i.dialog = true
	i.dialogWidth, i.dialogHeight = width, height
	i.dialogGrid = tview.NewGrid().AddItem(content, 1, 1, 1, 1, 0, 0, true)
	i.pages.AddPage("dialog", modalLayer{i.dialogGrid}, true, true)
	i.app.SetFocus(content)
}

func (i *ide) dismiss() {
	if i.pickerCancel != nil {
		i.pickerCancel()
		i.pickerCancel = nil
	}
	i.pickerField = nil
	i.pages.RemovePage("dialog")
	i.dialog = false
	if i.diffCancel != nil {
		i.diffCancel()
		i.diffCancel = nil
		i.app.SetFocus(i.changeList)
		return
	}
	if i.active != nil {
		i.app.SetFocus(i.active.editor)
	}
}

func (i *ide) prompt(title, label, value string, accept func(string)) {
	form := tview.NewForm().AddInputField(label, value, 0, nil, nil)
	input := form.GetFormItem(0).(*tview.InputField)
	submit := func() {
		text := strings.TrimSpace(input.GetText())
		if text == "" {
			return
		}
		i.dismiss()
		accept(text)
	}
	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEnter {
			submit()
			return nil
		}
		return event
	})
	form.AddButton("확인", submit).AddButton("취소", i.dismiss)
	form.SetBorder(true).SetTitle(" " + title + " ")
	i.showDialog(form, 76, 9)
}

func (i *ide) choose(title, text string, buttons []string, selected func(int)) {
	modal := tview.NewModal().SetText(text).AddButtons(buttons)
	modal.SetTitle(" " + title + " ")
	modal.SetDoneFunc(func(index int, _ string) {
		i.dismiss()
		selected(index)
	})
	i.showDialog(modal, 76, 13)
}

func (i *ide) openPrompt() {
	i.prompt("파일 열기 · 새 경로는 새 파일로 열립니다", "파일 경로", "", func(path string) {
		if err := i.openFile(path); err != nil {
			i.notify(err.Error())
		}
	})
}

func (i *ide) fileMenu() {
	list := tview.NewList().ShowSecondaryText(true)
	list.SetBorder(true).SetTitle(" 파일과 프로젝트 · Esc 닫기 ")
	for _, action := range i.fileActions() {
		list.AddItem(action.name, action.shortcut, 0, func() { i.dismiss(); action.run() })
	}
	i.showDialog(list, 76, 29)
}

type ideAction struct {
	name, shortcut string
	run            func()
}

func (i *ide) fileActions() []ideAction {
	return []ideAction{
		{"파일 열기", "Ctrl+O", i.openPrompt},
		{"빠른 파일 열기", "Ctrl+P · 파일명 일부로 찾기", i.quickOpen},
		{"프로젝트 전체 검색", "열린 문서의 편집 내용 포함", func() { i.openDock("검색") }},
		{"코드 자동완성", "Ctrl+K", func() { i.askLanguageServer("completion", false) }},
		{"함수 정의로 이동", "Ctrl+] · F12", func() { i.askLanguageServer("definition", false) }},
		{"참조 찾기", "언어 서버 · references", func() { i.askLanguageServer("references", false) }},
		{"심볼 이름 변경", "언어 서버 · rename", i.renameSymbol},
		{"새 파일", "Ctrl+N", i.newFile},
		{"저장", "Ctrl+S", func() { i.save(i.active, "", nil) }},
		{"다른 이름으로 저장", "현재 파일 경로를 새 이름으로 변경", func() {
			d := i.active
			i.prompt("다른 이름으로 저장", "파일 경로", d.disk.path, func(path string) { i.save(d, i.resolve(path), nil) })
		}},
		{"파일 닫기", "Ctrl+W", i.closeFile},
		{"찾기와 바꾸기", "Ctrl+H", func() { i.findPrompt(true) }},
		{"실행 작업", "F5 · 프로젝트 명령과 AI CLI", i.taskMenu},
		{"웹 미리보기", "live preview · 저장 시 자동 갱신", i.openWebPreview},
		{"웹 미리보기 종료", "stop preview", i.stopWebPreview},
		{"현재 Go 테스트 실행", "go test · 커서가 있는 테스트 함수", i.runCurrentTest},
		{"테스트 결과", "test results", func() { i.openDock("테스트 결과") }},
		{"다음 오류로 이동", "Alt+↓", func() { i.nextProblem(1) }},
		{"이전 오류로 이동", "Alt+↑", func() { i.nextProblem(-1) }},
		{"프로젝트 셸", "F7 · exit로 IDE에 복귀", func() { i.runTask(shellTask()) }},
		{"프로젝트 설정 열기", ".galaxy-ide.json", i.openConfig},
		{"설정과 파일 목록 새로 읽기", "들여쓰기와 실행 작업 적용", i.reloadConfig},
		{"도움말", "F1", i.help},
		{"종료", "Ctrl+Q", func() { i.quit(0) }},
	}
}

func (i *ide) commandPalette() {
	actions := i.fileActions()
	field := tview.NewInputField().SetLabel(" 명령 검색  ")
	list := tview.NewList().ShowSecondaryText(true).SetHighlightFullLine(true)
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(field, 1, 0, true).AddItem(list, 0, 1, false)
	box.SetBorder(true).SetTitle(" 명령 팔레트 · Enter 실행 · Esc 닫기 ")
	var matches []ideAction
	filter := func(value string) {
		list.Clear()
		matches = matches[:0]
		words := strings.Fields(strings.ToLower(value))
		for _, action := range actions {
			label := strings.ToLower(action.name + " " + action.shortcut)
			matchesAll := true
			for _, word := range words {
				matchesAll = matchesAll && strings.Contains(label, word)
			}
			if matchesAll {
				matches = append(matches, action)
				list.AddItem(tview.Escape(action.name), tview.Escape(action.shortcut), 0, nil)
			}
		}
		if len(matches) == 0 {
			list.AddItem("일치하는 명령 없음", "", 0, nil)
		}
	}
	selectAction := func(index int) {
		if index >= 0 && index < len(matches) {
			action := matches[index]
			i.dismiss()
			action.run()
		}
	}
	list.SetSelectedFunc(func(index int, _, _ string, _ rune) { selectAction(index) })
	field.SetChangedFunc(filter).SetInputCapture(func(e *tcell.EventKey) *tcell.EventKey {
		switch e.Key() {
		case tcell.KeyEnter:
			selectAction(list.GetCurrentItem())
		case tcell.KeyDown:
			list.SetCurrentItem(min(list.GetItemCount()-1, list.GetCurrentItem()+1))
		case tcell.KeyUp:
			list.SetCurrentItem(max(0, list.GetCurrentItem()-1))
		default:
			return e
		}
		return nil
	})
	filter("")
	i.showDialog(box, 82, 24)
	i.pickerField = field
}

func (i *ide) findPrompt(replace bool) {
	form := tview.NewForm().AddInputField("찾을 글자", i.query, 0, nil, nil)
	query := form.GetFormItem(0).(*tview.InputField)
	find := func() { i.query = query.GetText(); i.dismiss(); i.findNext() }
	if replace {
		form.AddInputField("바꿀 글자", "", 0, nil, nil)
		replacement := form.GetFormItem(1).(*tview.InputField)
		form.AddButton("하나 바꾸기", func() {
			i.query = query.GetText()
			value := replacement.GetText()
			i.dismiss()
			selected, _, _ := i.active.editor.GetSelection()
			if selected != i.query && !i.findNext() {
				return
			}
			if i.query != "" {
				_, start, end := i.active.editor.GetSelection()
				i.active.editor.Replace(start, end, value)
			}
		})
		form.AddButton("모두 바꾸기", func() {
			i.query = query.GetText()
			value := replacement.GetText()
			i.dismiss()
			if i.query == "" {
				return
			}
			editor := i.active.editor
			text := editor.GetText()
			count := strings.Count(text, i.query)
			if count > 0 {
				editor.Replace(0, len(text), strings.ReplaceAll(text, i.query, value))
			}
			i.notify(fmt.Sprintf("%d개를 바꿨습니다 · Ctrl+Z로 되돌리기", count))
		})
	}
	form.AddButton("다음 찾기", find).AddButton("취소", i.dismiss)
	query.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if !replace && event.Key() == tcell.KeyEnter {
			find()
			return nil
		}
		return event
	})
	form.SetBorder(true).SetTitle(" 찾기 · 대소문자 구분 ")
	i.showDialog(form, 78, 12)
}

func (i *ide) findNext() bool {
	if i.query == "" {
		i.notify("Ctrl+F로 찾을 글자를 입력하세요")
		return false
	}
	editor := i.active.editor
	text := editor.GetText()
	_, _, end := editor.GetSelection()
	index := strings.Index(text[end:], i.query)
	if index >= 0 {
		index += end
	} else {
		index = strings.Index(text, i.query)
	}
	if index < 0 {
		i.notify("일치하는 글자가 없습니다")
		return false
	}
	editor.Select(index, index+len(i.query))
	i.app.SetFocus(editor)
	i.notify("찾았습니다 · F3 다음 결과 · Ctrl+H 바꾸기")
	return true
}

func (i *ide) openConfig() {
	path := filepath.Join(i.root, ".galaxy-ide.json")
	if err := i.openFile(path); err != nil {
		i.notify(err.Error())
		return
	}
	if !i.active.disk.exists && i.active.editor.GetText() == "" {
		example := config{IndentSize: 4, Tasks: []task{{Name: "Git 상태", Args: []string{"git", "status", "--short"}}}}
		data, _ := json.MarshalIndent(example, "", "  ")
		i.active.editor.SetText(string(data)+"\n", false)
	}
}

func (i *ide) reloadConfig() {
	c, err := loadConfig(i.root)
	if err != nil {
		i.notify(err.Error())
		return
	}
	i.config = c
	tview.TabSize = c.IndentSize
	i.refreshTree()
	i.notify("설정과 파일 목록을 다시 읽었습니다")
}

func (i *ide) help() {
	text := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetText(`[::b]Galaxy IDE[-::-]

파일을 열고 바로 입력하면 됩니다. 방향키로 이동하고 마우스로 선택할 수 있습니다.

[#8edbc6]파일[-]
Ctrl+O          파일 열기 또는 새 경로 입력
Ctrl+P          파일명 일부로 빠른 열기
하단 검색       프로젝트 전체 검색 · 결과를 누르거나 Enter로 이동
하단 변경사항   Git 변경 목록 · 파일을 눌러 비교 · Esc로 목록 복귀
Ctrl+K          커서 옆 자동완성 · ↑/↓ 선택 · Tab 적용 · Esc 닫기
Ctrl+] / F12    커서에 있는 함수의 정의로 이동
상단 명령 / F10 명령 팔레트 · 참조 찾기 · 심볼 이름 변경
Ctrl+N          새 파일
Ctrl+S          저장
Ctrl+W          현재 탭 닫기
Ctrl+Q          종료 · 미저장 변경 확인
F4              다음 탭
Ctrl+B / F6     파일 탐색기와 편집기 사이 이동
상단 탐색기 / F2 파일 탐색기 표시 전환
Ctrl+E / F9     파일 메뉴 · 다른 이름으로 저장 · 설정

[#8edbc6]편집[-]
Shift+방향키    선택
Alt+← / Alt+→   코드 가로 이동 · 편집기 아래 ◀/▶ 버튼
Alt+↑ / Alt+↓   이전 오류 / 다음 오류로 이동
Ctrl+A          전체 선택
Ctrl+C / X / V  IDE 내부 복사 / 잘라내기 / 붙여넣기
Ctrl+Z / Y      실행 취소 / 다시 실행
Ctrl+F / F3     찾기 / 다음 결과
Ctrl+H          바꾸기
Ctrl+G          행으로 이동
Tab / Enter     들여쓰기 / 자동 들여쓰기

Android 클립보드는 터미널의 붙여넣기 메뉴 또는 Ctrl+Shift+V를 사용하세요.

[#8edbc6]실행과 확장[-]
Ctrl+R          프로젝트 build 작업 · 현재 언어 실행 또는 웹 미리보기
실행 메뉴       프로젝트 test 작업 선택
명령 팔레트     커서가 있는 Go 테스트만 실행 · 하단 테스트 결과에서 위치 이동
F5              실행 작업 메뉴
상단 AI         Codex · Claude Code · AGY를 코드 옆에서 실행
Ctrl+J          문제 목록 · Enter로 오류 위치 이동
Ctrl+T          AI 창이 열려 있으면 코드 / AI 사이 전환
                AI 창이 닫혀 있으면 하단 터미널 / 편집기 전환
F8              기존 프로젝트 터미널 전체 화면 확대 / 복귀
F7              전체 화면 프로젝트 셸
.galaxy-ide.json의 tasks에 명령을 추가하면 F5에 표시됩니다.
{file}은 저장한 파일 경로, {root}는 프로젝트 폴더입니다.
명령은 선택했을 때만 실행됩니다. 셸 문법은 bash -lc를 명시하세요.

UTF-8 텍스트 8 MiB까지 지원합니다. LF/CRLF, BOM, 파일 권한을 보존합니다.
외부에서 변경된 파일은 저장 전에 확인합니다.

Esc로 편집기에 돌아갑니다.`)
	text.SetBorder(true).SetTitle(" 도움말 · 스크롤 가능 ")
	i.showDialog(text, 92, 32)
}
