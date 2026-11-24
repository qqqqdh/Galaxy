package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"galaxy.local/tools/internal/ui"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type fileManager struct {
	*ui.Screen
	list               *tview.List
	location, status   *tview.TextView
	dir, source        string
	entries            []os.DirEntry
	move, hidden, busy bool
}

func main() {
	flag.Usage = func() { fmt.Fprintln(flag.CommandLine.Output(), "사용법: galaxy-files [폴더]") }
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	dir := "."
	if flag.NArg() == 1 {
		dir = flag.Arg(0)
	}
	m := &fileManager{Screen: ui.New(), list: ui.List(), location: tview.NewTextView(), status: tview.NewTextView()}
	m.location.SetTextColor(tview.Styles.SecondaryTextColor).SetWrap(false)
	m.status.SetTextColor(tview.Styles.SecondaryTextColor)
	m.list.SetBorder(true).SetTitle(" Galaxy 파일 ")
	m.list.SetSelectedFunc(func(_ int, _, _ string, _ rune) { m.actions() })
	bar := ui.Bar(
		ui.Action{Label: "상위", Run: m.up},
		ui.Action{Label: "작업", Run: m.actions},
		ui.Action{Label: "폴더", Run: m.locations},
		ui.Action{Label: "종료", Run: m.quit},
	)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(m.location, 1, 0, false).
		AddItem(m.list, 0, 1, true).
		AddItem(bar, 1, 0, false).
		AddItem(m.status, 2, 0, false)
	m.SetMain(layout, m.list)
	m.OnEscape(m.quit)
	m.list.SetInputCapture(m.keys)
	if err := m.load(dir, ""); err != nil {
		fmt.Fprintln(os.Stderr, "galaxy-files:", err)
		os.Exit(1)
	}
	m.message("파일을 선택하거나 터치하면 작업 메뉴가 열립니다.")
	if err := m.App.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "galaxy-files:", err)
		os.Exit(1)
	}
}

func (m *fileManager) message(text string) {
	if m.source != "" {
		action := "복사"
		if m.move {
			action = "이동"
		}
		text += "\n" + action + " 대기 · " + filepath.Base(m.source)
	}
	lines := strings.SplitN(text, "\n", 2)
	for i, line := range lines {
		lines[i] = ui.Text(line)
	}
	m.status.SetText(strings.Join(lines, "\n"))
}

func (m *fileManager) load(path, selected string) error {
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return err
	}
	if !m.hidden {
		entries = slices.DeleteFunc(entries, func(entry os.DirEntry) bool {
			return strings.HasPrefix(entry.Name(), ".")
		})
	}
	slices.SortStableFunc(entries, func(a, b os.DirEntry) int {
		if a.IsDir() != b.IsDir() {
			if a.IsDir() {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name()), strings.ToLower(b.Name()))
	})
	m.dir, m.entries = path, entries
	m.location.SetText(ui.Text(path))
	m.list.Clear()
	index := 0
	for i, entry := range entries {
		label := entry.Name()
		switch {
		case entry.IsDir():
			label += "/"
		case entry.Type()&os.ModeSymlink != 0:
			label += " →"
		}
		m.list.AddItem(ui.Text(label), "", 0, nil)
		if entry.Name() == selected {
			index = i
		}
	}
	m.list.SetCurrentItem(index)
	return nil
}

func (m *fileManager) goTo(path, selected string) {
	if m.busy {
		return
	}
	if err := m.load(path, selected); err != nil {
		m.message(err.Error())
		return
	}
	m.message(fmt.Sprintf("%d개 항목", len(m.entries)))
}

func (m *fileManager) up() {
	m.goTo(filepath.Dir(m.dir), filepath.Base(m.dir))
}

func (m *fileManager) quit() {
	if !m.busy {
		m.App.Stop()
	}
}

func (m *fileManager) locations() {
	if m.busy {
		return
	}
	m.Menu("폴더 이동",
		ui.Action{Label: "경로 입력", Run: func() {
			m.Prompt("폴더 이동", "경로", m.dir, func(path string) error {
				home, _ := os.UserHomeDir()
				switch {
				case path == "~":
					path = home
				case strings.HasPrefix(path, "~/"):
					path = filepath.Join(home, path[2:])
				case path != "" && !filepath.IsAbs(path):
					path = filepath.Join(m.dir, path)
				}
				if path == "" {
					return fmt.Errorf("폴더 경로를 입력하세요")
				}
				return m.load(path, "")
			})
		}},
		ui.Action{Label: "홈 폴더", Run: func() {
			home, err := os.UserHomeDir()
			if err != nil {
				m.message(err.Error())
				return
			}
			m.goTo(home, "")
		}},
		ui.Action{Label: "Android 파일 전송", Run: func() { m.goTo(transferDir, "") }},
	)
}

func (m *fileManager) selected() string {
	index := m.list.GetCurrentItem()
	if index < 0 || index >= len(m.entries) {
		return ""
	}
	return m.entries[index].Name()
}

func (m *fileManager) keys(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		m.up()
	case tcell.KeyF2:
		if name := m.selected(); name != "" {
			m.goTo(filepath.Join(m.dir, name), "")
		}
	case tcell.KeyF5:
		m.goTo(m.dir, m.selected())
	case tcell.KeyCtrlC:
		m.stage(m.selected(), false)
	case tcell.KeyCtrlV:
		m.paste()
	default:
		return event
	}
	return nil
}

func (m *fileManager) stage(name string, move bool) {
	if m.busy || name == "" {
		return
	}
	m.source, m.move = filepath.Join(m.dir, name), move
	m.message("대상 폴더로 이동한 뒤 붙여넣기를 선택하세요.")
}

func (m *fileManager) perform(message, selected string, action func() error, finished func()) {
	m.busy = true
	m.message(message + "…")
	go func() {
		err := action()
		m.App.QueueUpdateDraw(func() {
			m.busy = false
			refreshErr := m.load(m.dir, selected)
			switch {
			case err != nil:
				m.message(err.Error())
			case refreshErr != nil:
				m.message(refreshErr.Error())
			default:
				if finished != nil {
					finished()
				}
				m.message(message + " 완료")
			}
		})
	}()
}

func (m *fileManager) paste() {
	if m.busy || m.source == "" {
		return
	}
	source, move := m.source, m.move
	name := filepath.Base(source)
	target := filepath.Join(m.dir, name)
	if move {
		m.perform("이동", name, func() error { return renameNew(source, target) }, func() { m.source = "" })
	} else {
		m.perform("복사", name, func() error { return copyEntry(source, target) }, nil)
	}
}

func (m *fileManager) actions() {
	if m.busy {
		return
	}
	name := m.selected()
	path := filepath.Join(m.dir, name)
	initial := []ui.Action{}
	if name != "" {
		entry := m.entries[m.list.GetCurrentItem()]
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			initial = append(initial, ui.Action{Label: "폴더 열기", Run: func() { m.goTo(path, "") }})
		}
		initial = append(initial,
			ui.Action{Label: "복사", Run: func() { m.stage(name, false) }},
			ui.Action{Label: "이동", Run: func() { m.stage(name, true) }},
			ui.Action{Label: "이름 변경", Run: func() {
				m.Prompt("이름 변경", "이름", name, func(newName string) error {
					if err := validName(newName); err != nil {
						return err
					}
					if newName == name {
						return nil
					}
					if err := renameNew(path, filepath.Join(m.dir, newName)); err != nil {
						return err
					}
					m.goTo(m.dir, newName)
					return nil
				})
			}},
		)
		if base, ok := archiveBase(name); ok && entry.Type().IsRegular() {
			initial = append(initial, ui.Action{Label: "압축 풀기", Run: func() {
				m.Prompt("압축 풀기", "새 폴더", base, func(folder string) error {
					if err := validName(folder); err != nil {
						return err
					}
					target := filepath.Join(m.dir, folder)
					m.perform("압축 풀기", folder, func() error { return extractArchive(path, target) }, nil)
					return nil
				})
			}})
		}
		initial = append(initial, ui.Action{Label: "삭제", Run: func() {
			m.Confirm(ui.Text(name)+" 항목을 영구 삭제할까요?", func() {
				m.perform("삭제", "", func() error { return os.RemoveAll(path) }, nil)
			})
		}})
	}
	if m.source != "" {
		initial = append([]ui.Action{{Label: "여기에 붙여넣기", Run: m.paste}}, initial...)
	}
	initial = append(initial,
		ui.Action{Label: "새 폴더", Run: func() {
			m.Prompt("새 폴더", "이름", "", func(folder string) error {
				if err := validName(folder); err != nil {
					return err
				}
				if err := os.Mkdir(filepath.Join(m.dir, folder), 0755); err != nil {
					return err
				}
				m.goTo(m.dir, folder)
				return nil
			})
		}},
		ui.Action{Label: "숨김 파일 표시 전환", Run: func() {
			m.hidden = !m.hidden
			m.goTo(m.dir, name)
		}},
		ui.Action{Label: "새로 고침", Run: func() { m.goTo(m.dir, name) }},
	)
	if name == "" {
		name = "파일 작업"
	}
	m.Menu(ui.Text(name), initial...)
}
