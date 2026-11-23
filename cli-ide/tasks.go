package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rivo/tview"
)

func shellTask() task {
	args := []string{"/bin/bash", "-i"}
	if _, err := os.Stat("/etc/galaxy-bashrc"); err == nil {
		args = []string{"/bin/bash", "--noprofile", "--rcfile", "/etc/galaxy-bashrc", "-i"}
	}
	return task{Name: "프로젝트 셸", Args: args}
}

func nativeLanguage(path string) string {
	ext := filepath.Ext(path)
	switch strings.ToLower(ext) {
	case ".c":
		if ext == ".c" {
			return "c"
		}
		return "c++"
	case ".cpp", ".cc", ".cxx":
		return "c++"
	}
	return ""
}

func nativeBuild(path string) (task, bool) {
	language := nativeLanguage(path)
	if language == "" {
		return task{}, false
	}
	compiler := "c++"
	if language == "c" {
		compiler = "cc"
	}
	return task{Name: "프로젝트 빌드 후 실행", Args: []string{compiler, "-Wall", "-Wextra", "-x", language, "{file}"}, buildAndRun: true}, true
}

type nativeProject struct {
	root    string
	shallow bool
	ignore  []string
}

func (i *ide) nativeProject() nativeProject {
	root, file := i.root, i.active.disk.path
	home, _ := os.UserHomeDir()
	rel, err := filepath.Rel(root, file)
	if err != nil || !filepath.IsLocal(rel) || root == home || root == filepath.Dir(root) {
		root = filepath.Dir(file)
	}
	return nativeProject{root: root, shallow: root == home || root == filepath.Dir(root), ignore: i.config.Ignore}
}

func (p nativeProject) includes(path string) bool {
	rel, err := filepath.Rel(p.root, path)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if p.shallow && len(parts) > 1 {
		return false
	}
	for _, part := range parts {
		if strings.HasPrefix(part, ".") || part == "build" || strings.HasPrefix(part, "cmake-build-") || slices.Contains(p.ignore, part) {
			return false
		}
	}
	return true
}

func (p nativeProject) walkSources(add func(string)) error {
	return filepath.WalkDir(p.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || path == p.root {
			return err
		}
		if entry.IsDir() && (p.shallow || !p.includes(path)) {
			return filepath.SkipDir
		}
		if entry.Type().IsRegular() && nativeLanguage(path) != "" && p.includes(path) {
			add(path)
		}
		return nil
	})
}

func (p nativeProject) buildArgs(active string) ([]string, error) {
	var cFiles, cppFiles []string
	add := func(path string) {
		switch nativeLanguage(path) {
		case "c":
			cFiles = append(cFiles, path)
		case "c++":
			cppFiles = append(cppFiles, path)
		}
	}
	add(active)
	err := p.walkSources(func(path string) {
		if path != active {
			add(path)
		}
	})
	if err != nil {
		return nil, err
	}
	args := []string{"cc", "-Wall", "-Wextra", "-I", p.root, "-I", filepath.Join(p.root, "include")}
	if len(cFiles) > 0 {
		args = append(args, "-x", "c")
		args = append(args, cFiles...)
	}
	if len(cppFiles) > 0 {
		args[0] = "c++"
		args = append(args, "-x", "c++")
		args = append(args, cppFiles...)
	}
	return args, nil
}

var assistantTasks = []task{
	{Name: "Codex", Args: []string{"codex", "--no-daemon", "-c", "tui.disable_paste_burst=true"}, assistant: true},
	{Name: "Claude Code", Args: []string{"claude"}, assistant: true},
	{Name: "AGY", Args: []string{"agy"}, assistant: true},
}

func (i *ide) assistantMenu() {
	list := tview.NewList().ShowSecondaryText(false)
	list.SetBorder(true).SetTitle(" AI · 코드와 나란히 보기 ")
	if i.assistantName != "" {
		list.AddItem(i.assistantName+" 대화 보기", "", 0, func() {
			i.dismiss()
			i.showAssistant()
		})
	}
	for _, entry := range assistantTasks {
		list.AddItem(entry.Name, "", 0, func() {
			i.dismiss()
			i.runTask(entry)
		})
	}
	i.showDialog(list, 40, list.GetItemCount()+3)
}

func (i *ide) taskMenu() {
	list := tview.NewList().ShowSecondaryText(true)
	list.SetBorder(true).SetTitle(" 실행 작업 · 선택한 명령을 프로젝트 폴더에서 실행 ")
	tasks := append([]task{shellTask()}, assistantTasks...)
	tasks = append(tasks, task{Name: "Git 상태", Args: []string{"git", "status", "--short", "--branch"}})
	if t, ok := fileTask(i.active.disk.path, i.nativeProject().root); ok {
		tasks = append([]task{t}, tasks...)
	}
	if build, ok := nativeBuild(i.active.disk.path); ok {
		check := task{Name: "현재 파일 문법 검사", Args: append([]string{build.Args[0], "-fsyntax-only"}, build.Args[1:]...)}
		tasks = append([]task{build, check}, tasks...)
	}
	var grouped []task
	for _, group := range []string{"build", "test"} {
		if t, ok := i.config.groupTask(group); ok {
			grouped = append(grouped, t)
		}
	}
	for _, t := range i.config.Tasks {
		if t.Group == "" {
			tasks = append(tasks, t)
		}
	}
	tasks = append(grouped, tasks...)
	for _, entry := range tasks {
		entry := entry
		description := strings.Join(entry.Args, " ")
		if entry.Group == "build" {
			description = "Ctrl+R · 프로젝트 빌드 · " + description
		} else if entry.Group == "test" {
			description = "프로젝트 테스트 · " + description
		} else if entry.buildAndRun {
			description = "프로젝트의 C/C++ 소스를 함께 컴파일하고 링크"
		}
		list.AddItem(entry.Name, description, 0, func() {
			i.dismiss()
			i.runTask(entry)
		})
	}
	i.showDialog(list, 90, min(2*len(tasks)+4, 28))
}

func (i *ide) defaultBuild() (task, bool) {
	if t, ok := i.config.groupTask("build"); ok {
		return t, true
	}
	if t, ok := nativeBuild(i.active.disk.path); ok {
		return t, true
	}
	return fileTask(i.active.disk.path, i.nativeProject().root)
}

func (i *ide) runTask(t task) {
	if t.assistant {
		i.openAssistant(t)
		return
	}
	if t.buildAndRun || t.Group != "" {
		for _, d := range i.documents {
			if !d.dirty() {
				continue
			}
			if t.buildAndRun {
				if d != i.active && !i.nativeProject().includes(d.disk.path) {
					continue
				}
			} else {
				rel, err := filepath.Rel(i.root, d.disk.path)
				usesActive := d == i.active && slices.ContainsFunc(t.Args, func(s string) bool { return strings.Contains(s, "{file}") })
				if !usesActive && (d.disk.path == "" || err != nil || !filepath.IsLocal(rel)) {
					continue
				}
			}
			i.choose("실행 전 저장", d.name()+"의 변경 내용을 저장하고 계속할까요?", []string{"저장 후 계속", "취소"}, func(index int) {
				if index == 0 {
					i.save(d, "", func() {
						if t.Group != "" && d.disk.path == filepath.Join(i.root, ".galaxy-ide.json") {
							c, err := loadConfig(i.root)
							if err != nil {
								i.notify(err.Error())
								return
							}
							next, ok := c.groupTask(t.Group)
							if !ok {
								i.notify("설정에서 해당 작업이 제거됐습니다")
								return
							}
							i.runTask(next)
							return
						}
						i.runTask(t)
					})
				}
			})
			return
		}
		i.executeTask(t)
		return
	}
	i.guard(i.active, func() { i.executeTask(t) })
}

func (i *ide) executeTask(t task) {
	var args []string
	var err error
	if t.buildAndRun {
		args, err = i.nativeProject().buildArgs(i.active.disk.path)
	} else {
		args, err = expandArgs(t.Args, i.active.disk.path, i.root)
	}
	if err != nil {
		i.notify(err.Error())
		return
	}
	name := filepath.FromSlash(args[0])
	if !filepath.IsAbs(name) && strings.ContainsRune(name, filepath.Separator) {
		name = filepath.Join(i.root, name)
	}
	program, err := exec.LookPath(name)
	if err != nil {
		i.notify("명령을 찾을 수 없습니다 · " + args[0] + "를 Ubuntu에 설치하세요")
		return
	}
	var buildIssues []problem
	var runErr error
	suspended := i.app.Suspend(func() {
		// The foreground task and IDE share a terminal process group during Suspend.
		interrupts := make(chan os.Signal, 1)
		signal.Notify(interrupts, os.Interrupt)
		defer signal.Stop(interrupts)
		run := func(executable string, args ...string) error {
			compiling := t.buildAndRun && executable == program
			if compiling {
				args = append([]string{"-fdiagnostics-format=json"}, args...)
			}
			cmd := exec.Command(executable, args...)
			cmd.Dir = i.root
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			if compiling {
				var diagnostics bytes.Buffer
				cmd.Stderr = &diagnostics
				err := cmd.Run()
				if err != nil && unsupportedJSON(diagnostics.Bytes()) {
					diagnostics.Reset()
					plain := exec.Command(executable, args[1:]...)
					plain.Dir, plain.Stdin, plain.Stdout, plain.Stderr = cmd.Dir, cmd.Stdin, cmd.Stdout, &diagnostics
					err = plain.Run()
				}
				buildIssues = parseProblems(diagnostics.Bytes(), i.root, "")
				for _, issue := range buildIssues {
					fmt.Fprintln(os.Stderr, issue.label())
				}
				return err
			}
			return cmd.Run()
		}
		if t.buildAndRun {
			runErr = buildAndRun(program, args[1:], run)
		} else {
			runErr = run(program, args[1:]...)
		}
		if t.Group != "" || t.Name != "프로젝트 셸" {
			fmt.Println()
			if runErr != nil {
				fmt.Println("실행 결과", runErr)
			}
			fmt.Print("Enter를 누르면 Galaxy IDE로 돌아갑니다.")
			_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		}
	})
	if !suspended {
		i.notify("터미널을 전환하지 못했습니다")
		return
	}
	i.refreshDocuments()
	i.refreshTree()
	if t.buildAndRun {
		i.cancelCheck()
		i.showProblems(buildIssues, "빌드 진단 없음")
		if len(buildIssues) > 0 {
			i.dockOpen = true
			i.dock.SwitchToPage("문제")
		}
	}
	if runErr != nil {
		i.notify(t.Name + " · " + runErr.Error())
	} else {
		i.notify(t.Name + "에서 돌아왔습니다")
	}
}

func buildAndRun(compiler string, args []string, run func(string, ...string) error) error {
	dir, err := os.MkdirTemp("", "galaxy-ide-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	binary := filepath.Join(dir, "program")
	if err := run(compiler, append(args, "-o", binary)...); err != nil {
		return fmt.Errorf("빌드 실패: %w", err)
	}
	fmt.Println("빌드 성공 · 프로그램 실행")
	return run(binary)
}

func (i *ide) refreshDocuments() {
	for _, d := range i.documents {
		if d.disk.path == "" || d.dirty() {
			continue
		}
		current, err := readDocument(d.disk.path)
		if err != nil || !current.exists || current.digest == d.disk.digest {
			continue
		}
		_, start, end := d.editor.GetSelection()
		d.disk = current
		d.editor.SetText(current.text, false).Select(start, end)
		i.updateTab(d)
	}
}
