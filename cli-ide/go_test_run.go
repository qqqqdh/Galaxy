package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/rivo/tview"
)

func goTestAt(path, source string, offset int) string {
	if !strings.HasSuffix(path, "_test.go") {
		return ""
	}
	files := token.NewFileSet()
	file, _ := parser.ParseFile(files, path, source, parser.SkipObjectResolution)
	if file == nil {
		return ""
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil || fn.Type.TypeParams != nil ||
			fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
			continue
		}
		suffix, isTest := strings.CutPrefix(fn.Name.Name, "Test")
		first, _ := utf8.DecodeRuneInString(suffix)
		if !isTest || unicode.IsLower(first) {
			continue
		}
		params := fn.Type.Params.List
		if len(params) != 1 || len(params[0].Names) > 1 {
			continue
		}
		pointer, ok := params[0].Type.(*ast.StarExpr)
		if !ok {
			continue
		}
		arg := pointer.X
		if qualified, ok := arg.(*ast.SelectorExpr); ok {
			arg = qualified.Sel
		}
		if name, ok := arg.(*ast.Ident); !ok || name.Name != "T" {
			continue
		}
		start := fn.Pos()
		if fn.Doc != nil {
			start = fn.Doc.Pos()
		}
		if files.Position(start).Offset <= offset && offset < files.Position(fn.End()).Offset {
			return fn.Name.Name
		}
	}
	return ""
}

func (i *ide) runCurrentTest() {
	d := i.active
	_, offset, _ := d.editor.GetSelection()
	name := goTestAt(d.disk.path, d.editor.GetText(), offset)
	if name == "" {
		i.notify("Go의 _test.go 파일에서 테스트 함수 안에 커서를 두세요")
		return
	}
	i.runGoTest(filepath.Dir(d.disk.path), name)
}

func (i *ide) runGoTest(dir, name string) {
	for _, d := range i.documents {
		path := d.disk.path
		if !d.dirty() || path == "" {
			continue
		}
		base := filepath.Base(path)
		rel, err := filepath.Rel(filepath.Dir(path), dir)
		packageFile := filepath.Dir(path) == dir && filepath.Ext(path) == ".go"
		moduleFile := err == nil && filepath.IsLocal(rel) && (base == "go.mod" || base == "go.sum" || base == "go.work" || base == "go.work.sum")
		if !packageFile && !moduleFile {
			continue
		}
		i.choose("테스트 실행 전 저장", d.name()+"의 변경 내용을 저장하고 실행할까요?", []string{"저장 후 실행", "취소"}, func(index int) {
			if index == 0 {
				i.save(d, "", func() { i.runGoTest(dir, name) })
			}
		})
		return
	}
	if i.testCancel != nil {
		i.testCancel()
	}
	ctx, cancel := context.WithCancel(i.languageLife)
	i.testCancel = cancel
	i.testProblems = nil
	i.testList.Clear()
	i.testList.AddItem(tview.Escape(name+" 실행 중…"), "", 0, nil)
	i.openDock("테스트 결과")
	go func() {
		lines, err := executeGoTest(ctx, dir, name)
		if ctx.Err() != nil {
			return
		}
		i.app.QueueUpdateDraw(func() {
			if ctx.Err() != nil {
				return
			}
			cancel()
			i.testCancel = nil
			i.testList.Clear()
			result := name + " 통과"
			if err != nil {
				result = name + " 실패 · " + err.Error()
			}
			i.testList.AddItem(tview.Escape(result), "", 0, nil)
			firstLink := 0
			for _, line := range lines {
				p := line
				var jump func()
				if p.file != "" {
					if err != nil {
						i.testProblems = append(i.testProblems, p)
					}
					jump = func() { i.jumpProblem(p) }
					if firstLink == 0 {
						firstLink = i.testList.GetItemCount()
					}
				}
				i.testList.AddItem(tview.Escape(p.message), "", 0, jump)
			}
			i.testList.SetCurrentItem(firstLink)
			i.notify(result)
		})
	}()
}

var goTestLocation = regexp.MustCompile(`^(.+?\.go):([0-9]+)(?::([0-9]+))?:\s`)

func goTestLine(line, dir string) problem {
	p := problem{message: line}
	if m := goTestLocation.FindStringSubmatch(line); m != nil {
		path := filepath.Clean(m[1])
		if !filepath.IsAbs(path) {
			path = filepath.Join(dir, path)
		}
		row, _ := strconv.Atoi(m[2])
		column, _ := strconv.Atoi(m[3])
		if row > 0 {
			p.file, p.line, p.first, p.kind = path, row, max(1, column), "error"
		}
	}
	return p
}

func executeGoTest(ctx context.Context, dir, name string) ([]problem, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-json", "-count=1", "-run", "^"+regexp.QuoteMeta(name)+"$", ".")
	isolateTestCommand(cmd)
	cmd.Dir = dir
	output, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		_ = output.Close()
		return nil, err
	}
	stopOutput := context.AfterFunc(ctx, func() { _ = output.Close() })
	defer stopOutput()
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 4096), maxFileSize)
	var lines []problem
	plainLines, links := 0, 0
	ran := false
	for scanner.Scan() {
		var event struct{ Action, Test, Output string }
		text := scanner.Text()
		if json.Unmarshal(scanner.Bytes(), &event) == nil {
			ran = ran || event.Action == "run" && event.Test == name
			text = event.Output
		}
		for line := range strings.SplitSeq(text, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			item := goTestLine(line, dir)
			switch {
			case item.file != "" && links < 400:
				links++
				lines = append(lines, item)
			case item.file == "" && plainLines < 100:
				plainLines++
				lines = append(lines, item)
			}
		}
	}
	if scanner.Err() != nil {
		_, _ = io.Copy(io.Discard, output)
	}
	if err := cmd.Wait(); err != nil {
		return lines, err
	}
	if err := scanner.Err(); err != nil {
		return lines, err
	}
	if !ran {
		return lines, fmt.Errorf("테스트가 빌드 조건에 포함되는지 확인하세요")
	}
	return lines, nil
}
