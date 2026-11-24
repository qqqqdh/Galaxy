package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"galaxy.local/tools/internal/ui"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
	"golang.org/x/sys/unix"
)

type manager struct {
	*ui.Screen
	root, state        string
	profiles           []profile
	list               *tview.List
	log, link, message *tview.TextView
	currentID, logs    string
	labels             []string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "galaxy-servers:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 5 && os.Args[1] == "--worker" {
		return serve(serverRun{os.Args[2]}, os.Args[3], os.Args[4])
	}
	flag.Usage = func() { fmt.Fprintln(flag.CommandLine.Output(), "사용법: galaxy-servers [프로젝트 폴더]") }
	flag.Parse()
	if flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}
	project := "."
	if flag.NArg() == 1 {
		project = flag.Arg(0)
	}
	root, state, err := openProject(project)
	if err != nil {
		return err
	}
	lock, err := takeLock(filepath.Join(state, "manager.lock"))
	if errors.Is(err, unix.EWOULDBLOCK) {
		return errors.New("이 프로젝트의 서버 관리자 화면이 이미 열려 있습니다")
	}
	if err != nil {
		return err
	}
	defer lock.Close()
	profiles, err := readProfiles(state)
	if err != nil {
		return err
	}
	m := &manager{
		Screen: ui.New(), root: root, state: state, profiles: profiles,
		list: ui.List(), log: tview.NewTextView(), link: tview.NewTextView().SetDynamicColors(true).SetWrap(false),
		message: tview.NewTextView(),
	}
	m.list.SetBorder(true).SetTitle(" Galaxy 서버 ")
	m.log.SetBorder(true).SetTitle(" 최근 로그 ")
	m.log.ScrollToEnd()
	m.message.SetTextColor(tview.Styles.SecondaryTextColor)
	m.list.SetSelectedFunc(func(_ int, _, _ string, _ rune) { m.actions() })
	m.list.SetInputCapture(m.keys)
	m.log.SetInputCapture(m.keys)
	header := tview.NewTextView().SetWrap(false).SetText(ui.Text(root))
	bar := ui.Bar(
		ui.Action{Label: "추가", Run: func() { m.edit(-1) }},
		ui.Action{Label: "작업", Run: m.actions},
		ui.Action{Label: "로그", Run: func() { m.App.SetFocus(m.log) }},
		ui.Action{Label: "종료", Run: m.App.Stop},
	)
	layout := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 1, 0, false).
		AddItem(m.list, 0, 2, true).
		AddItem(m.log, 0, 3, false).
		AddItem(m.link, 1, 0, false).
		AddItem(bar, 1, 0, false).
		AddItem(m.message, 2, 0, false)
	m.SetMain(layout, m.list)
	m.OnEscape(m.App.Stop)
	m.refreshList(0)
	m.show("추가로 서버를 등록하세요 · 화면 종료 후에도 현재 Ubuntu 세션에서 계속 실행됩니다.")

	done := make(chan struct{})
	defer close(done)
	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				m.App.QueueUpdateDraw(m.refresh)
			}
		}
	}()
	return m.App.Run()
}

func (m *manager) keys(event *tcell.EventKey) *tcell.EventKey {
	switch event.Key() {
	case tcell.KeyF2:
		m.edit(-1)
	case tcell.KeyF5:
		m.actions()
	case tcell.KeyTab:
		if m.log.HasFocus() {
			m.App.SetFocus(m.list)
		} else {
			m.App.SetFocus(m.log)
		}
	default:
		return event
	}
	return nil
}

func (m *manager) show(message string) { m.message.SetText(ui.Text(message)) }

func (m *manager) runFor(p profile) serverRun {
	return serverRun{dir: filepath.Join(m.state, p.ID)}
}

func (m *manager) refreshList(selection int) {
	m.list.SetChangedFunc(nil).Clear()
	m.labels = make([]string, len(m.profiles))
	for i, p := range m.profiles {
		m.labels[i] = m.label(p)
		m.list.AddItem(m.labels[i], "", 0, nil)
	}
	m.list.SetCurrentItem(selection)
	m.list.SetChangedFunc(func(index int, _, _ string, _ rune) { m.refreshDetails(index) })
	m.refreshDetails(m.list.GetCurrentItem())
}

func (m *manager) label(p profile) string {
	return ui.Text(m.runFor(p).status() + "  ·  " + p.Name)
}

func (m *manager) refresh() {
	for i, p := range m.profiles {
		label := m.label(p)
		if m.labels[i] != label {
			m.labels[i] = label
			m.list.SetItemText(i, label, "")
		}
	}
	m.refreshDetails(m.list.GetCurrentItem())
}

func (m *manager) refreshDetails(i int) {
	if i < 0 || i >= len(m.profiles) {
		m.currentID, m.logs = "", ""
		m.log.SetText("서버를 추가하면 여기에 출력이 표시됩니다.")
		m.link.SetText("")
		return
	}
	p := m.profiles[i]
	text := m.runFor(p).logs()
	if m.currentID != p.ID || m.logs != text {
		if m.currentID != p.ID {
			m.log.ScrollToEnd()
		}
		m.currentID, m.logs = p.ID, text
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = ui.Text(line)
		}
		visible := strings.Join(lines, "\n")
		if visible == "" {
			visible = "아직 출력이 없습니다."
		}
		m.log.SetText(visible)
	}
	if validBrowserURL(p.URL) {
		m.link.SetText("브라우저  [#8edbc6::u:" + p.URL + "]" + p.URL + "[-::-:-]")
	} else {
		m.link.SetText("")
	}
}

func (m *manager) edit(index int) {
	p := profile{}
	if index >= 0 {
		p = m.profiles[index]
	}
	directory := p.Dir
	if directory == "" {
		directory = m.root
	}
	form := tview.NewForm().SetItemPadding(0)
	form.SetBorder(true).SetTitle(" 서버 설정 ")
	form.AddInputField("이름", p.Name, 0, nil, nil).
		AddInputField("실행 명령", p.Command, 0, nil, nil).
		AddInputField("작업 폴더", directory, 0, nil, nil).
		AddInputField("브라우저 주소", p.URL, 0, nil, nil)
	form.AddButton("저장", func() {
		value := func(field int) string { return form.GetFormItem(field).(*tview.InputField).GetText() }
		updated, err := settings(m.root, p, value(0), value(1), value(2), value(3))
		selection := index
		next := slices.Clone(m.profiles)
		if err == nil {
			if index < 0 {
				selection = len(next)
				next = append(next, updated)
			} else {
				next[index] = updated
			}
			err = saveProfiles(m.state, next)
		}
		if err != nil {
			form.SetTitle(" " + tview.Escape(ui.Text(err.Error())) + " ")
			return
		}
		m.profiles = next
		m.refreshList(selection)
		m.CloseOverlay()
		m.show("서버 설정을 저장했습니다.")
	})
	form.AddButton("취소", m.CloseOverlay)
	m.Overlay(form)
}

func (m *manager) actions() {
	i := m.list.GetCurrentItem()
	if i < 0 || i >= len(m.profiles) {
		m.edit(-1)
		return
	}
	p := m.profiles[i]
	r := m.runFor(p)
	running, err := r.running()
	if err != nil {
		m.show(err.Error())
		return
	}
	report := func(message string, err error) {
		if err != nil {
			message = err.Error()
		}
		m.show(message)
		m.refresh()
	}
	if running {
		m.Menu(ui.Text(p.Name), ui.Action{Label: "서버 중지", Run: func() {
			report("서버 중지를 요청했습니다.", r.requestStop())
		}})
		return
	}
	m.Menu(ui.Text(p.Name),
		ui.Action{Label: "서버 시작", Run: func() {
			report("서버를 시작합니다. 브라우저를 열기 전에 로그를 확인하세요.", r.start(p))
		}},
		ui.Action{Label: "설정 편집", Run: func() { m.edit(i) }},
		ui.Action{Label: "등록 삭제", Run: func() {
			m.Confirm(ui.Text(p.Name)+" 설정과 로그를 삭제할까요?", func() {
				next := slices.Delete(slices.Clone(m.profiles), i, i+1)
				if err := saveProfiles(m.state, next); err != nil {
					m.show(err.Error())
					return
				}
				m.profiles = next
				m.refreshList(max(0, min(i, len(next)-1)))
				report("서버 등록을 삭제했습니다.", os.RemoveAll(r.dir))
			})
		}},
	)
}
