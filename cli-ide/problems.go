package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"github.com/rivo/uniseg"
)

type gccPoint struct {
	File   string
	Line   int
	Column int `json:"byte-column"`
}

type problem struct {
	file, kind, message string
	line, first, last   int // GCC uses one-based, inclusive byte columns.
}

var textLocation = regexp.MustCompile(`^(.+?):([0-9]+):([0-9]+): (fatal error|error|warning|note): (.*)$`)

func unsupportedJSON(output []byte) bool {
	message := string(output)
	return strings.Contains(message, "diagnostics-format") && (strings.Contains(message, "invalid") || strings.Contains(message, "unrecognized") || strings.Contains(message, "unknown"))
}

func parseProblems(output []byte, dir, stdin string) (out []problem) {
	for _, line := range bytes.Split(bytes.TrimSpace(output), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var entries []struct {
			Kind, Message string
			Locations     []struct{ Caret, Start, Finish gccPoint }
		}
		if json.Unmarshal(line, &entries) != nil {
			if m := textLocation.FindStringSubmatch(string(line)); m != nil {
				path := m[1]
				if path == "<stdin>" {
					path = stdin
				} else if !filepath.IsAbs(path) {
					path = filepath.Join(dir, path)
				}
				row, _ := strconv.Atoi(m[2])
				col, _ := strconv.Atoi(m[3])
				out = append(out, problem{file: path, line: row, first: col, last: col, kind: m[4], message: m[5]})
			} else if len(line) > 0 && line[0] != ' ' && line[0] != '\t' {
				out = append(out, problem{kind: "error", message: string(line)})
			}
			continue
		}
		for _, entry := range entries {
			p := problem{kind: entry.Kind, message: entry.Message}
			if len(entry.Locations) > 0 {
				l := entry.Locations[0]
				p.file, p.line, p.first, p.last = l.Caret.File, l.Caret.Line, l.Caret.Column, l.Caret.Column
				if l.Start.Line == p.line && l.Start.Column > 0 {
					p.first = l.Start.Column
				}
				if l.Finish.Line == p.line {
					p.last = max(p.first, l.Finish.Column)
				}
				switch {
				case p.file == "<stdin>":
					p.file = stdin
				case p.file != "" && !filepath.IsAbs(p.file):
					p.file = filepath.Join(dir, p.file)
				}
			}
			out = append(out, p)
		}
	}
	return
}

func (p problem) label() string {
	prefix := map[string]string{"error": "오류", "fatal error": "오류", "warning": "경고", "note": "참고"}[p.kind]
	if prefix == "" {
		prefix = p.kind
	}
	if p.file != "" {
		return fmt.Sprintf("%s  %s:%d:%d  %s", prefix, filepath.Base(p.file), p.line, p.first, p.message)
	}
	return prefix + "  " + p.message
}

// Translate compiler byte positions to the same grapheme cells drawn by TextArea.
func (p problem) mark(lines []string, tab int) (colorSpan, bool) {
	if p.line < 1 || p.line > len(lines) || p.first < 1 {
		return colorSpan{}, false
	}
	c := tcell.NewHexColor(0xff656d)
	if p.kind == "warning" {
		c = tcell.NewHexColor(0xffc857)
	}
	col, start, end := 0, -1, -1
	g := uniseg.NewGraphemes(lines[p.line-1])
	for g.Next() {
		left, right := g.Positions()
		width := g.Width()
		if g.Str() == "\t" {
			width = tab
		}
		if right > p.first-1 && left < max(p.first, p.last) {
			if start < 0 {
				start = col
			}
			end = col + width
		}
		col += width
	}
	if start < 0 {
		start = col
	}
	return colorSpan{p.line - 1, start, max(start+1, end), c}, true
}

func (i *ide) showProblems(items []problem, empty string) {
	i.problemList.Clear()
	i.problemButton.SetLabel(fmt.Sprintf("문제 (%d)", len(items)))
	for _, d := range i.documents {
		d.syntax.marks = nil
	}
	cache := map[*openDocument][]string{}
	for _, p := range items {
		for _, d := range i.documents {
			if d.disk.path != p.file || p.file == "" {
				continue
			}
			if cache[d] == nil {
				cache[d] = strings.Split(d.editor.GetText(), "\n")
			}
			if mark, ok := p.mark(cache[d], tview.TabSize); ok {
				d.syntax.marks = append(d.syntax.marks, mark)
			}
		}
		i.problemList.AddItem(tview.Escape(p.label()), "", 0, func() {
			if p.file == "" || p.line < 1 {
				return
			}
			if i.active.disk.path != p.file {
				if err := i.openFile(p.file); err != nil {
					i.notify(err.Error())
					return
				}
			}
			lines := strings.Split(i.active.editor.GetText(), "\n")
			offset := 0
			for row := 0; row < min(p.line-1, len(lines)); row++ {
				offset += len(lines[row]) + 1
			}
			if p.line <= len(lines) {
				offset += min(max(0, p.first-1), len(lines[p.line-1]))
			}
			offset = min(offset, i.active.editor.GetTextLength())
			i.active.editor.Select(offset, offset)
			i.app.SetFocus(i.active.editor)
		})
	}
	if len(items) == 0 {
		i.problemList.AddItem(empty, "", 0, nil)
	}
}

func (i *ide) cancelCheck() {
	if i.timer != nil {
		i.timer.Stop()
	}
	if i.cancel != nil {
		i.cancel()
	}
}

func (i *ide) checkActive() {
	if !i.live || i.active == nil {
		return
	}
	i.cancelCheck()
	d := i.active
	build, ok := nativeBuild(d.disk.path)
	if !ok {
		i.showProblems(nil, "C/C++ 소스 파일을 열면 자동 검사합니다")
		return
	}
	i.showProblems(nil, "검사 중…")
	ctx, cancel := context.WithCancel(context.Background())
	i.cancel = cancel
	source, path, root := d.editor.GetText(), d.disk.path, i.nativeProject().root
	i.timer = time.AfterFunc(500*time.Millisecond, func() {
		deadline, stop := context.WithTimeout(ctx, 15*time.Second)
		defer stop()
		args := []string{"-fsyntax-only", "-fdiagnostics-format=json", "-Wall", "-Wextra", "-x", nativeLanguage(path), "-iquote", filepath.Dir(path), "-I", root, "-I", filepath.Join(root, "include"), "-"}
		compile := func(flags []string) ([]byte, error) {
			cmd := exec.CommandContext(deadline, build.Args[0], flags...)
			cmd.Dir = filepath.Dir(path)
			cmd.Stdin = strings.NewReader(source)
			return cmd.CombinedOutput()
		}
		output, err := compile(args)
		if err != nil && unsupportedJSON(output) {
			output, err = compile(append(args[:1], args[2:]...))
		}
		if ctx.Err() != nil {
			return
		}
		items := parseProblems(output, filepath.Dir(path), path)
		if err != nil && len(items) == 0 {
			items = []problem{{kind: "error", message: "검사 실패 · " + err.Error()}}
		}
		i.app.QueueUpdateDraw(func() {
			if ctx.Err() == nil {
				i.showProblems(items, "문제 없음")
			}
		})
	})
}
