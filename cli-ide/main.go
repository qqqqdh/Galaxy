package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

const version = "0.1.9"

func main() {
	showVersion := flag.Bool("version", false, "버전 표시")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Galaxy IDE — 터미널에서 사용하는 프로젝트 편집기\n\n사용법: galaxy-ide [프로젝트 폴더 또는 파일]\n\nCtrl+S 저장 · Ctrl+O 열기 · Ctrl+Q 종료 · F1 도움말")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Println("Galaxy IDE " + version)
		return
	}
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	path := "."
	if flag.NArg() == 1 {
		path = flag.Arg(0)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		fail(err)
	}
	root, file := path, ""
	info, err := os.Stat(path)
	if os.IsNotExist(err) || (err == nil && !info.IsDir()) {
		root, file = filepath.Dir(path), path
	} else if err != nil {
		fail(err)
	}
	config, err := loadConfig(root)
	if err != nil {
		fail(err)
	}
	ide := newIDE(root, config)
	ide.live = true
	state := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(state) {
		home, err := os.UserHomeDir()
		if err != nil {
			fail(err)
		}
		state = filepath.Join(home, ".local", "state")
	}
	count, recoveryErr := ide.recoverDrafts(filepath.Join(state, "galaxy-ide", "drafts"))
	if file != "" {
		opened := false
		for _, d := range ide.documents {
			if d.disk.path == file {
				ide.activate(d)
				opened = true
				break
			}
		}
		if !opened {
			if err := ide.openFile(file); err != nil {
				fail(err)
			}
		}
	} else if count == 0 {
		ide.newFile()
	}
	if count > 0 {
		ide.notify(fmt.Sprintf("미저장 문서 %d개를 복구했습니다 · Ctrl+S로 저장", count))
	}
	if recoveryErr != nil {
		ide.notify("자동 복구 확인 필요 · " + recoveryErr.Error())
	}
	if err := ide.app.Run(); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "Galaxy IDE:", err)
	os.Exit(1)
}
