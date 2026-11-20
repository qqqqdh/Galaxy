package main

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/tview"
)

func (i *ide) clearHints() {
	if i.hintTimer != nil {
		i.hintTimer.Stop()
		i.hintTimer = nil
	}
	if i.cancelHint != nil {
		i.cancelHint()
		i.cancelHint = nil
	}
	if i.acceptHint != nil {
		i.acceptHint = nil
		i.dock.SwitchToPage(i.priorDock)
		i.dockOpen = i.priorDockOpen
	}
}
func identStart(text string, at int) int {
	for at > 0 {
		r, n := utf8.DecodeLastRuneInString(text[:at])
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			break
		}
		at -= n
	}
	return at
}
func (i *ide) scheduleCompletion(d *openDocument) {
	i.clearHints()
	if toolsFor(d.disk.path).id == "" {
		return
	}
	ctx, cancel := context.WithCancel(i.languageLife)
	i.cancelHint = cancel
	i.hintTimer = time.AfterFunc(350*time.Millisecond, func() {
		if ctx.Err() != nil {
			return
		}
		i.app.QueueUpdateDraw(func() {
			if ctx.Err() == nil && i.active == d && !i.dialog && d.editor.HasFocus() {
				i.askLanguageServer("completion", true)
			}
		})
	})
}
func (i *ide) askLanguageServer(method string, auto bool) {
	i.clearHints()
	d := i.active
	if d == nil || toolsFor(d.disk.path).id == "" {
		i.notify("언어 서버를 지원하는 소스 파일을 여세요")
		return
	}
	text := d.editor.GetText()
	_, at, end := d.editor.GetSelection()
	if auto && (at != end || (at-identStart(text, at) < 2 && !strings.HasSuffix(text[:at], ".") && !strings.HasSuffix(text[:at], "->") && !strings.HasSuffix(text[:at], "::"))) {
		return
	}
	ctx, cancel := context.WithCancel(i.languageLife)
	i.cancelHint = cancel
	tools := toolsFor(d.disk.path)
	files := i.languageFiles(tools.server)
	p := i.nativeProject()
	path := d.disk.path
	if !auto {
		i.notify(tools.server + " 조회 중…")
	}
	go func() {
		data, err := i.languages.query(i.languageLife, ctx, p, files, path, lspPosition(text, at), method)
		if ctx.Err() != nil {
			return
		}
		i.app.QueueUpdateDraw(func() {
			if ctx.Err() != nil || i.active != d || i.dialog || !d.editor.HasFocus() {
				return
			}
			_, a, b := d.editor.GetSelection()
			if a != at || b != end || d.editor.GetText() != text {
				return
			}
			if err != nil {
				if !auto {
					i.notify(tools.server + " · " + err.Error())
				}
				return
			}
			if method == "definition" {
				i.jumpDefinition(data)
				return
			}
			var result struct{ Items []completion }
			if err := json.Unmarshal(data, &result); err != nil {
				if err = json.Unmarshal(data, &result.Items); err != nil {
					i.notify(err.Error())
					return
				}
			}
			prefix := strings.ToLower(text[identStart(text, at):at])
			if prefix != "" && tools.server != "clangd" {
				result.Items = slices.DeleteFunc(result.Items, func(c completion) bool {
					label := c.FilterText
					if label == "" {
						label = strings.TrimSpace(c.Label)
					}
					return !strings.HasPrefix(strings.ToLower(label), prefix)
				})
			}
			if len(result.Items) == 0 {
				if !auto {
					i.notify(tools.server + " 자동완성 후보 없음")
				}
				return
			}
			i.hints.Clear()
			i.priorDock, _ = i.dock.GetFrontPage()
			i.priorDockOpen = i.dockOpen
			i.hintOffset = at
			i.acceptHint = func(index int) {
				if index < 0 || index >= len(result.Items) {
					return
				}
				_, a, b := d.editor.GetSelection()
				i.clearHints()
				i.app.SetFocus(d.editor)
				if i.active != d || a != at || b != end || d.editor.GetText() != text {
					i.notify("커서 위치가 바뀌었습니다")
					return
				}
				item := result.Items[index]
				start, finish := identStart(text, at), end
				value := item.InsertText
				if item.TextEdit != nil {
					r := item.TextEdit.Range
					if r == nil {
						r = item.TextEdit.Replace
					}
					if r == nil {
						return
					}
					var ok bool
					start, ok = bytePosition(text, r.Start)
					if !ok {
						return
					}
					finish, ok = bytePosition(text, r.End)
					if !ok || finish < start {
						return
					}
					value = item.TextEdit.NewText
				} else if value == "" {
					value = strings.TrimSpace(item.Label)
				}
				if item.InsertTextFormat==2 {
     expanded,a,b,ok:=webSnippet(value)
     if ok {
      d.editor.Replace(start,finish,expanded)
      d.editor.Select(start+a,start+b)
     }else{d.editor.Replace(start,finish,strings.TrimSpace(item.Label))}
    }else{d.editor.Replace(start,finish,value)}
				i.clearHints()
			}
			for n, item := range result.Items {
				i.hints.AddItem(tview.Escape(strings.TrimSpace(item.Label)+"  "+item.Detail), "", 0, func() {
					if i.acceptHint != nil {
						i.acceptHint(n)
					}
				})
			}
			i.dockOpen = true
			i.dock.SwitchToPage("자동완성")
			i.app.SetFocus(d.editor)
			i.notify(fmt.Sprintf("%s 자동완성 %d개 · Tab 적용 · ↓ 목록 · Esc 닫기", tools.server, len(result.Items)))
		})
	}()
}
func (i *ide) jumpDefinition(data json.RawMessage) {
	var locations []location
	if err := json.Unmarshal(data, &locations); err != nil {
		var one location
		if err = json.Unmarshal(data, &one); err != nil {
			i.notify(err.Error())
			return
		}
		locations = []location{one}
	}
	if len(locations) == 0 {
		i.notify("정의를 찾지 못했습니다")
		return
	}
	target := locations[0]
	if target.URI == "" && target.TargetURI != "" {
		target.URI, target.Range = target.TargetURI, target.TargetSelectionRange
	}
	path, err := uriPath(target.URI)
	if err == nil {
		err = i.openFile(path)
	}
	if err != nil {
		i.notify(err.Error())
		return
	}
	text := i.active.editor.GetText()
	start, ok := bytePosition(text, target.Range.Start)
	if !ok {
		i.notify("정의 위치가 변경됐습니다")
		return
	}
	end, valid := bytePosition(text, target.Range.End)
	if !valid || end < start {
		end = start
	}
	i.active.syntax.definitionSelection = &[2]int{start, end}
	i.notify(fmt.Sprintf("정의 · %s · %d행", i.active.name(), target.Range.Start.Line+1))
}

func (i *ide) languageFiles(server string) map[string]string {
	files := map[string]string{}
	for _, d := range i.documents {
		if toolsFor(d.disk.path).server == server {
			files[d.disk.path] = d.editor.GetText()
		}
	}
	return files
}
