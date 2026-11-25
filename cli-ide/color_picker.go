package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func (i *ide) pickColor(d *openDocument, literal colorLiteral) {
	text := d.editor.GetText()
	if i.active != d || literal.start < 0 || literal.end <= literal.start || literal.end > len(text) {
		return
	}
	old := text[literal.start:literal.end]
	components := literal.rgba
	sample := tview.NewTextView().SetDynamicColors(true)
	form := tview.NewForm()
	initial := fmt.Sprintf("#%02x%02x%02x", int(math.Round(components[0])), int(math.Round(components[1])), int(math.Round(components[2])))
	form.AddInputField("색상", initial, 12, nil, nil)
	form.AddInputField("불투명도(%)", strconv.FormatFloat(components[3]*100, 'f', -1, 64), 12, nil, nil)
	hex := form.GetFormItem(0).(*tview.InputField)
	opacity := form.GetFormItem(1).(*tview.InputField)
	selected := func() (rgba [4]float64, ok bool) {
		value := strings.TrimSpace(hex.GetText())
		matches := appendHexSwatch(nil, value, 0)
		percent, err := strconv.ParseFloat(strings.TrimSpace(opacity.GetText()), 64)
		if len(matches) != 1 || matches[0].edit.end != len(value) || len(value) != 4 && len(value) != 7 ||
			err != nil || math.IsNaN(percent) || percent < 0 || percent > 100 {
			return rgba, false
		}
		rgba = matches[0].edit.rgba
		rgba[3] = percent / 100
		return rgba, true
	}
	update := func(string) {
		rgba, ok := selected()
		if !ok {
			sample.SetText(" #RGB 또는 #RRGGBB와 0–100 사이의 불투명도를 입력하세요")
			return
		}
		color := compositeSwatch(rgba[0], rgba[1], rgba[2], rgba[3])
		sample.SetText(fmt.Sprintf(" 미리보기  [:#%06x]        [:-]", color.Hex()))
	}
	hex.SetChangedFunc(update)
	opacity.SetChangedFunc(update)
	presets := tview.NewFlex()
	for _, preset := range []struct{ name, rgb string }{
		{"빨강", "#e63946"}, {"주황", "#f97316"}, {"노랑", "#facc15"}, {"초록", "#22c55e"},
		{"파랑", "#2563eb"}, {"보라", "#9333ea"}, {"검정", "#000000"}, {"흰색", "#ffffff"},
	} {
		color := tcell.GetColor(preset.rgb)
		r, g, b := color.RGB()
		fg := tcell.ColorWhite
		if 299*r+587*g+114*b >= 150000 {
			fg = tcell.ColorBlack
		}
		button := tview.NewButton(preset.name).SetSelectedFunc(func() { hex.SetText(preset.rgb) })
		button.SetStyle(tcell.StyleDefault.Background(color).Foreground(fg))
		presets.AddItem(button, 0, 1, false)
	}
	form.AddButton("적용", func() {
		rgba, ok := selected()
		if !ok {
			update("")
			return
		}
		i.dismiss()
		if i.active != d || d.editor.GetText() != text {
			i.notify("색상 편집 중 파일 내용이 바뀌었습니다")
			return
		}
		if rgba == components {
			return
		}
		value := formatPickedColor(old, rgba)
		d.editor.Replace(literal.start, literal.end, value)
	}).AddButton("취소", i.dismiss)
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(presets, 1, 0, false).
		AddItem(sample, 1, 0, false).AddItem(form, 0, 1, true)
	box.SetBorder(true).SetTitle(" 색상 편집 ")
	update("")
	i.showDialog(box, 72, 12)
}

func formatPickedColor(original string, rgba [4]float64) string {
	r, g, b := int(rgba[0]), int(rgba[1]), int(rgba[2])
	if strings.HasPrefix(original, "#") {
		if rgba[3] == 1 {
			return fmt.Sprintf("#%02x%02x%02x", r, g, b)
		}
		return fmt.Sprintf("#%02x%02x%02x%02x", r, g, b, int(math.Round(rgba[3]*255)))
	}
	return fmt.Sprintf("rgba(%d, %d, %d, %s)", r, g, b, strconv.FormatFloat(rgba[3], 'f', -1, 64))
}
