package main

import "github.com/rivo/tview"

const assistantSplitWidth = 106

func (i *ide) makeAssistantPanel() *tview.Flex {
	i.assistant = &shellPane{Box: tview.NewBox(), app: i.app}
	i.assistantTitle = tview.NewTextView().SetTextColor(accent)
	i.assistantTitle.SetBackgroundColor(panel)
	bar := tview.NewFlex().AddItem(i.assistantTitle, 0, 1, false).
		AddItem(tview.NewButton("코드로").SetSelectedFunc(func() {
			i.app.SetFocus(i.active.editor)
		}), 9, 0, false).
		AddItem(tview.NewButton("숨기기").SetSelectedFunc(func() {
			i.assistantOpen = false
			i.app.SetFocus(i.active.editor)
		}), 9, 0, false)
	split := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(bar, 1, 0, false).AddItem(i.assistant, 0, 1, true)
	split.SetBorder(true).SetBorderColor(accent)
	return split
}

func (i *ide) showAssistant() {
	i.clearHints()
	i.assistantOpen = true
	i.app.SetFocus(i.assistant)
}

func (i *ide) openAssistant(job task) {
	start := func() {
		i.assistantName = job.Name
		i.assistantTitle.SetText(" AI · " + job.Name)
		i.showAssistant()
		// Measure the visible split before the child sees its initial terminal size.
		i.app.ForceDraw()
		i.assistant.run(i.root, job, func(err error) {
			i.assistantTitle.SetText(" AI · " + job.Name + " 종료")
			if err != nil {
				i.notify(job.Name + " · " + err.Error())
			}
			i.refreshDocuments()
			i.refreshTree()
		})
	}
	if i.assistant.file == nil {
		start()
		return
	}
	if i.assistantName == job.Name {
		i.showAssistant()
		return
	}
	i.choose("AI 전환", i.assistantName+"을 종료하고 "+job.Name+"을 열까요?", []string{"취소", "전환"}, func(index int) {
		if index == 1 {
			i.assistant.close()
			start()
		}
	})
}
