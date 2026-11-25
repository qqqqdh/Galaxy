//go:build terminal_worker_experiment

package main

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	experimentWidth  = 56
	experimentHeight = 42
	experimentKeys   = 100
	experimentRuns   = 3
)

type experimentScreen struct {
	tcell.SimulationScreen
	keyAt, keyHandled, keyDraw time.Time
	keys                       []experimentKeyTiming
	keysDone                   chan struct{}
	editorOnly, partial        bool
	partialFrames              int
}

func (s *experimentScreen) Clear() {
	s.partial = false
	if !s.keyAt.IsZero() {
		s.keyDraw = time.Now()
		w, h := s.Size()
		if s.editorOnly && w == 120 && h == experimentHeight {
			s.partial = true
			return
		}
	}
	s.SimulationScreen.Clear()
}

func (s *experimentScreen) Show() {
	s.SimulationScreen.Show()
	if !s.keyAt.IsZero() {
		shown := time.Now()
		s.keys = append(s.keys, experimentKeyTiming{
			queue: s.keyHandled.Sub(s.keyAt), edit: s.keyDraw.Sub(s.keyHandled),
			paint: shown.Sub(s.keyDraw), total: shown.Sub(s.keyAt),
		})
		s.keyAt = time.Time{}
		if len(s.keys) == experimentKeys {
			close(s.keysDone)
		}
	}
}

type experimentPane struct {
	*tview.Box
	cellAt func(int, int) *uv.Cell
}

func (p *experimentPane) Draw(screen tcell.Screen) {
	p.Box.DrawForSubclass(screen, p)
	x, y, w, h := p.GetInnerRect()
	for row := 0; row < h; row++ {
		for col := 0; col < w; col++ {
			cell := p.cellAt(col, row)
			if cell == nil || cell.Width == 0 || cell.Width > w-col {
				continue
			}
			text := []rune(cell.Content)
			if len(text) == 0 {
				text = []rune{' '}
			}
			style := cell.Style
			screen.SetContent(x+col, y+row, text[0], text[1:], tcell.StyleDefault.
				Foreground(termColor(style.Fg, foreground)).Background(termColor(style.Bg, background)).
				Bold(style.Attrs&uv.AttrBold != 0).Reverse(style.Attrs&uv.AttrReverse != 0).Underline(style.Underline != 0))
		}
	}
}

type experimentFrame struct {
	cells  [experimentWidth * experimentHeight]uv.Cell
	cursor uv.Position
}

func (f *experimentFrame) capture(em *vt.Emulator) {
	for row := range experimentHeight {
		for col := range experimentWidth {
			cell := em.CellAt(col, row)
			if cell == nil {
				f.cells[row*experimentWidth+col] = uv.Cell{}
			} else {
				f.cells[row*experimentWidth+col] = *cell
			}
		}
	}
	f.cursor = em.CursorPosition()
}

func (f *experimentFrame) cellAt(col, row int) *uv.Cell {
	return &f.cells[row*experimentWidth+col]
}

type experimentLoad struct {
	name     string
	streams  int
	bytes    int
	interval time.Duration
}

type experimentResult struct {
	keys      []experimentKeyTiming
	output    []experimentFrameTiming
	allocated uint64
	batches   int
}

type experimentKeyTiming struct {
	queue, edit, paint, total time.Duration
}

type experimentFrameTiming struct {
	parse, snapshot, queue, paint, total time.Duration
}

type experimentMode struct {
	name               string
	worker, editorOnly bool
}

func TestTerminalWorkerExperiment(t *testing.T) {
	t.Logf("platform=%s/%s cpus=%d GOMAXPROCS=%d runs=%d", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), experimentRuns)
	for _, load := range []experimentLoad{
		{"ai", 1, 256, 8 * time.Millisecond},
		{"burst", 1, 24 * 1024, 40 * time.Millisecond},
		{"three_bursts", 3, 24 * 1024, 40 * time.Millisecond},
	} {
		t.Run(load.name, func(t *testing.T) {
			compareTerminalExperiment(t, load, [2]experimentMode{{name: "ui"}, {name: "worker", worker: true}})
		})
	}
}

func TestTerminalEditorRedrawExperiment(t *testing.T) {
	t.Logf("platform=%s/%s cpus=%d GOMAXPROCS=%d runs=%d", runtime.GOOS, runtime.GOARCH, runtime.NumCPU(), runtime.GOMAXPROCS(0), experimentRuns)
	modes := [2]experimentMode{{name: "full_redraw", worker: true}, {name: "editor_only", worker: true, editorOnly: true}}
	for _, load := range []experimentLoad{
		{"idle", 1, 0, 40 * time.Millisecond},
		{"burst", 1, 24 * 1024, 40 * time.Millisecond},
		{"three_bursts", 3, 24 * 1024, 40 * time.Millisecond},
	} {
		t.Run(load.name, func(t *testing.T) { compareTerminalExperiment(t, load, modes) })
	}
}

func compareTerminalExperiment(t *testing.T, load experimentLoad, modes [2]experimentMode) {
	var results [2]experimentResult
	for run := range experimentRuns {
		for _, model := range []int{run % 2, 1 - run%2} {
			result := runTerminalExperiment(t, load, modes[model])
			results[model].keys = append(results[model].keys, result.keys...)
			results[model].output = append(results[model].output, result.output...)
			results[model].allocated += result.allocated
			results[model].batches += result.batches
		}
	}
	for model, result := range results {
		keyTotals := make([]time.Duration, 0, len(result.keys))
		keyQueues := make([]time.Duration, 0, len(result.keys))
		keyEdits := make([]time.Duration, 0, len(result.keys))
		keyPaints := make([]time.Duration, 0, len(result.keys))
		for _, key := range result.keys {
			keyTotals = append(keyTotals, key.total)
			keyQueues = append(keyQueues, key.queue)
			keyEdits = append(keyEdits, key.edit)
			keyPaints = append(keyPaints, key.paint)
		}
		totals := make([]time.Duration, 0, len(result.output))
		var sum experimentFrameTiming
		for _, frame := range result.output {
			totals = append(totals, frame.total)
			sum.parse += frame.parse
			sum.snapshot += frame.snapshot
			sum.queue += frame.queue
			sum.paint += frame.paint
		}
		mean := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) / float64(len(totals)) }
		fmt.Printf("RESULT load=%s model=%s keys=%d batches=%d key_p50_ms=%.3f key_p95_ms=%.3f key_p99_ms=%.3f key_queue_p95_ms=%.3f key_edit_p95_ms=%.3f key_paint_p95_ms=%.3f frame_p95_ms=%.3f parse_avg_ms=%.3f snapshot_avg_ms=%.3f queue_avg_ms=%.3f paint_avg_ms=%.3f alloc_mib_per_run=%.2f\n",
			load.name, modes[model].name, len(result.keys), result.batches,
			experimentPercentile(keyTotals, 50), experimentPercentile(keyTotals, 95), experimentPercentile(keyTotals, 99),
			experimentPercentile(keyQueues, 95), experimentPercentile(keyEdits, 95), experimentPercentile(keyPaints, 95), experimentPercentile(totals, 95),
			mean(sum.parse), mean(sum.snapshot), mean(sum.queue), mean(sum.paint),
			float64(result.allocated)/experimentRuns/(1<<20))
	}
}

func TestTerminalWorkerFrameExchange(t *testing.T) {
	runTerminalExperiment(t, experimentLoad{"concurrent", 3, 4 * 1024, 35 * time.Millisecond}, experimentMode{worker: true})
}

func TestTerminalEditorPartialRedraw(t *testing.T) {
	runTerminalExperiment(t, experimentLoad{"idle", 1, 0, 40 * time.Millisecond}, experimentMode{worker: true, editorOnly: true})
}

func TestTerminalScrollbackOrder(t *testing.T) {
	history := vt.NewScrollback(3)
	push := func(text string) {
		for _, r := range text {
			history.Push(uv.Line{{Content: string(r), Width: 1}, uv.EmptyCell})
		}
	}
	check := func(want string) {
		t.Helper()
		lines := history.Lines()
		expected := []rune(want)
		if len(lines) != len(expected) || history.Len() != len(lines) || history.Line(-1) != nil || history.Line(len(lines)) != nil {
			t.Fatalf("unexpected history bounds for %q", want)
		}
		for i, r := range expected {
			cell := history.CellAt(0, i)
			if len(lines[i]) != 1 || lines[i][0].Content != string(r) || cell == nil || cell.Content != string(r) {
				t.Fatalf("history line %d does not match %q", i, r)
			}
		}
	}
	push("abcd한")
	check("cd한")
	history.SetMaxLines(2)
	check("d한")
	push("bc")
	check("bc")
	history.SetMaxLines(4)
	push("de가")
	check("cde가")
	history.Clear()
	check("")
	push("한글")
	check("한글")
}

func TestTerminalScrollRegion(t *testing.T) {
	for _, test := range []struct {
		area  uv.Rectangle
		count int
		want  []string
	}{
		{uv.Rect(0, 0, 4, 5), 1, []string{"efgh", "ijkl", "mnop", "qrst", "...."}},
		{uv.Rect(0, 1, 4, 4), 2, []string{"abcd", "mnop", "qrst", "....", "...."}},
		{uv.Rect(1, 1, 2, 4), 1, []string{"abcd", "ejkh", "inol", "mrsp", "q..t"}},
	} {
		buffer := uv.NewBuffer(4, 5)
		for row, line := range []string{"abcd", "efgh", "ijkl", "mnop", "qrst"} {
			for col, r := range line {
				buffer.SetCell(col, row, &uv.Cell{Content: string(r), Width: 1})
			}
		}
		buffer.DeleteLineArea(test.area.Min.Y, test.count, &uv.Cell{Content: ".", Width: 1}, test.area)
		for row, want := range test.want {
			for col, r := range want {
				if cell := buffer.CellAt(col, row); cell == nil || cell.Content != string(r) {
					t.Fatalf("scroll area %v produced the wrong cell at %d,%d", test.area, col, row)
				}
			}
		}
	}
}

func runTerminalExperiment(t *testing.T, load experimentLoad, mode experimentMode) experimentResult {
	t.Helper()
	var result experimentResult
	screen := &experimentScreen{SimulationScreen: tcell.NewSimulationScreen("UTF-8"), keysDone: make(chan struct{}), editorOnly: mode.editorOnly}
	app := tview.NewApplication().SetScreen(screen)
	screen.SetSize(120, experimentHeight)
	front := vt.NewEmulator(experimentWidth, experimentHeight)
	defer front.Close()
	pane := &experimentPane{Box: tview.NewBox(), cellAt: front.CellAt}
	var frames *[2]experimentFrame
	if mode.worker {
		frames = new([2]experimentFrame)
		frames[0].capture(front)
		pane.cellAt = frames[0].cellAt
	}
	editor := tview.NewTextArea().SetWrap(false)
	root := tview.NewFlex().AddItem(editor, 0, 1, true).AddItem(pane, experimentWidth, 0, false)
	app.SetRoot(root, true).SetFocus(editor)
	app.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		screen.keyAt = event.When()
		screen.keyHandled = time.Now()
		return event
	})
	app.SetBeforeDrawFunc(func(target tcell.Screen) bool {
		if screen.partial {
			editor.Draw(target)
			screen.partialFrames++
			return true
		}
		return false
	})
	ready := make(chan struct{})
	var started sync.Once
	app.SetAfterDrawFunc(func(tcell.Screen) { started.Do(func() { close(ready) }) })
	appDone := make(chan error, 1)
	go func() { appDone <- app.Run() }()
	select {
	case <-ready:
	case err := <-appDone:
		t.Fatalf("IDE simulation exited before drawing: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("IDE simulation did not draw")
	}

	pattern := []byte("\x1b[38;5;75mAI output 한글 with ANSI colors\x1b[0m\r\n")
	payload := bytes.Repeat(pattern, (load.bytes+len(pattern)-1)/len(pattern))
	end := []byte("\x1b[2J\x1b[Hworker-output-end")
	const keyboardPeriod = 17 * time.Millisecond
	start := time.Now().Add(70 * time.Millisecond)
	finish := start.Add(experimentKeys * keyboardPeriod)
	var memoryBefore, memoryAfter runtime.MemStats
	runtime.ReadMemStats(&memoryBefore)
	var streams sync.WaitGroup
	for index := range load.streams {
		streams.Add(1)
		go func() {
			defer streams.Done()
			em := front
			if index != 0 {
				em = vt.NewEmulator(experimentWidth, experimentHeight)
				defer em.Close()
			}
			output := make(chan []byte, 8)
			go func() {
				defer close(output)
				for due := start; len(payload) > 0 && due.Before(finish); due = due.Add(load.interval) {
					time.Sleep(time.Until(due))
					for offset := 0; offset < len(payload); offset += terminalOutputChunk {
						output <- payload[offset:min(len(payload), offset+terminalOutputChunk)]
					}
				}
				output <- end
			}()
			// The UI keeps one frame while the worker writes the other one.
			nextFrame := 1
			relayTerminalOutput(output, func(batch []byte) {
				batchReady := time.Now()
				var timing experimentFrameTiming
				if mode.worker {
					_, _ = em.Write(batch)
					if index != 0 {
						return
					}
					parsed := time.Now()
					timing.parse = parsed.Sub(batchReady)
					frames[nextFrame].capture(em)
					timing.snapshot = time.Since(parsed)
				}
				queued := time.Now()
				app.QueueUpdate(func() {
					dequeued := time.Now()
					timing.queue = dequeued.Sub(queued)
					if !mode.worker {
						_, _ = em.Write(batch)
						timing.parse = time.Since(dequeued)
					}
					if index == 0 {
						if mode.worker {
							pane.cellAt = frames[nextFrame].cellAt
						}
						paint := time.Now()
						pane.Draw(screen)
						screen.Show()
						shown := time.Now()
						timing.paint = shown.Sub(paint)
						timing.total = shown.Sub(batchReady)
						result.output = append(result.output, timing)
					}
				})
				nextFrame ^= 1
			})
			if !strings.HasPrefix(em.String(), "worker-output-end") {
				t.Error("terminal did not parse its final output")
			}
		}()
	}
	go func() {
		for index := range experimentKeys {
			time.Sleep(time.Until(start.Add(time.Duration(index)*keyboardPeriod + 3*time.Millisecond)))
			app.QueueEvent(tcell.NewEventKey(tcell.KeyRune, rune('a'+index%26), tcell.ModNone))
		}
	}()
	select {
	case <-screen.keysDone:
	case <-time.After(10 * time.Second):
		t.Fatal("IDE simulation did not paint all key presses")
	}
	streams.Wait()
	app.QueueUpdate(func() {
		x, y, _, _ := pane.GetInnerRect()
		cells, width, _ := screen.GetContents()
		if editor.GetTextLength() != experimentKeys || string(cells[y*width+x].Runes) != "w" {
			t.Error("IDE simulation lost input or painted stale terminal output")
		}
		if mode.editorOnly && screen.partialFrames != experimentKeys {
			t.Errorf("painted %d editor-only frames, want %d", screen.partialFrames, experimentKeys)
		}
		if mode.worker && frames[0].cursor != front.CursorPosition() && frames[1].cursor != front.CursorPosition() {
			t.Error("terminal frame lost the final cursor position")
		}
	})
	app.Stop()
	if err := <-appDone; err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&memoryAfter)
	result.keys = screen.keys
	result.allocated = memoryAfter.TotalAlloc - memoryBefore.TotalAlloc
	result.batches = len(result.output)
	return result
}

func experimentPercentile(samples []time.Duration, percentile int) float64 {
	slices.Sort(samples)
	index := (len(samples)*percentile+99)/100 - 1
	return float64(samples[index]) / float64(time.Millisecond)
}
