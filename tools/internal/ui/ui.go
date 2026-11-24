package ui

import (
	"strings"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

type Screen struct {
	App   *tview.Application
	pages *tview.Pages
	focus tview.Primitive
}

func New() *Screen {
	tview.Styles.PrimitiveBackgroundColor = tcell.GetColor("#111722")
	tview.Styles.ContrastBackgroundColor = tcell.GetColor("#192332")
	tview.Styles.MoreContrastBackgroundColor = tcell.GetColor("#31465b")
	tview.Styles.PrimaryTextColor = tcell.GetColor("#dde6ef")
	tview.Styles.SecondaryTextColor = tcell.GetColor("#8edbc6")
	tview.Styles.TitleColor = tview.Styles.SecondaryTextColor
	tview.Styles.BorderColor = tcell.GetColor("#536a80")
	app := tview.NewApplication().EnableMouse(true).EnablePaste(true)
	pages := tview.NewPages()
	app.SetRoot(pages, true)
	return &Screen{App: app, pages: pages}
}

func (s *Screen) SetMain(root, focus tview.Primitive) {
	s.focus = focus
	s.pages.AddPage("main", root, true, true)
	s.App.SetFocus(focus)
}

func (s *Screen) Overlay(view tview.Primitive) {
	s.pages.AddAndSwitchToPage("overlay", view, true)
	s.App.SetFocus(view)
}

func (s *Screen) CloseOverlay() {
	s.pages.RemovePage("overlay")
	s.pages.ShowPage("main")
	s.App.SetFocus(s.focus)
}

func (s *Screen) OnEscape(quit func()) {
	s.App.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape {
			if name, _ := s.pages.GetFrontPage(); name == "overlay" {
				s.CloseOverlay()
			} else {
				quit()
			}
			return nil
		}
		return event
	})
}

type Action struct {
	Label string
	Run   func()
}

func (s *Screen) Menu(title string, items ...Action) {
	menu := List()
	menu.SetBorder(true).SetTitle(" " + tview.Escape(title) + " ")
	for _, action := range items {
		menu.AddItem(action.Label, "", 0, func() {
			s.CloseOverlay()
			action.Run()
		})
	}
	menu.AddItem("돌아가기", "", 0, s.CloseOverlay)
	s.Overlay(menu)
}

func (s *Screen) Confirm(message string, action func()) {
	modal := tview.NewModal().SetText(tview.Escape(message)).AddButtons([]string{"취소", "확인"}).
		SetDoneFunc(func(index int, _ string) {
			s.CloseOverlay()
			if index == 1 {
				action()
			}
		})
	s.Overlay(modal)
}

func (s *Screen) Prompt(title, label, initial string, action func(string) error) {
	form := tview.NewForm().SetItemPadding(0)
	form.SetBorder(true).SetTitle(" " + title + " ")
	form.AddInputField(label, initial, 0, nil, nil)
	form.AddButton("확인", func() {
		value := form.GetFormItem(0).(*tview.InputField).GetText()
		if err := action(value); err != nil {
			form.SetTitle(" " + tview.Escape(Text(err.Error())) + " ")
			return
		}
		s.CloseOverlay()
	})
	form.AddButton("취소", s.CloseOverlay)
	s.Overlay(form)
}

func List() *tview.List {
	return tview.NewList().ShowSecondaryText(false).SetUseStyleTags(false, false).
		SetWrapAround(false).SetHighlightFullLine(true)
}

func Bar(actions ...Action) *tview.Flex {
	bar := tview.NewFlex()
	for _, action := range actions {
		bar.AddItem(tview.NewButton(action.Label).SetSelectedFunc(action.Run), 0, 1, false)
	}
	return bar
}

// Render untrusted names and logs without terminal control characters.
func Text(value string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\n', '\r', '\t':
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return '�'
		}
		return r
	}, value)
}
