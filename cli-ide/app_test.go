package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func TestNativeProjectSources(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"main.c", "src/연산 file.c", "lib/math.C", "include/math.h", "build/cache.c", ".cache/temp.c", "ignored/test.cpp"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	project := nativeProject{root: root, ignore: []string{"ignored"}}
	main := filepath.Join(root, "main.c")
	args, err := project.buildArgs(main)
	expected := []string{"c++", "-Wall", "-Wextra", "-I", root, "-I", filepath.Join(root, "include"), "-x", "c", main, filepath.Join(root, "src/연산 file.c"), "-x", "c++", filepath.Join(root, "lib/math.C")}
	if err != nil || !slices.Equal(args, expected) {
		t.Fatalf("mixed project: %q, %v", args, err)
	}
	project.shallow = true
	args, err = project.buildArgs(main)
	expected[0] = "cc"
	if err != nil || !slices.Equal(args, expected[:10]) {
		t.Fatalf("flat project: %q, %v", args, err)
	}
}

func TestNativeBuildAndRun(t *testing.T) {
	for ext, compiler := range map[string]string{".c": "cc", ".C": "c++", ".cpp": "c++", ".cc": "c++", ".cxx": "c++", ".h": ""} {
		job, ok := nativeBuild("한 글" + ext)
		if ok != (compiler != "") || ok && (!job.buildAndRun || job.Args[0] != compiler) {
			t.Fatalf("unexpected task for %s: %+v", ext, job)
		}
	}
	failure := errors.New("compile failed")
	for _, compileErr := range []error{nil, failure} {
		var executable string
		ran := false
		err := buildAndRun("cc", []string{"my file.c"}, func(program string, args ...string) error {
			if program == "cc" {
				if len(args) != 3 || args[0] != "my file.c" || args[1] != "-o" {
					t.Fatal("incorrect compiler arguments", args)
				}
				executable = args[2]
				return compileErr
			}
			if program != executable || len(args) != 0 {
				t.Fatal("did not execute the fresh build", program, args)
			}
			ran = true
			return nil
		})
		if ran != (compileErr == nil) || !errors.Is(err, compileErr) {
			t.Fatalf("compile error %v, ran %t, returned %v", compileErr, ran, err)
		}
		if _, err := os.Stat(filepath.Dir(executable)); !os.IsNotExist(err) {
			t.Fatal("build directory not cleaned up", err)
		}
	}
}

func TestSyntaxInEditor(t *testing.T) {
	root := t.TempDir()
	i := newIDE(root, config{IndentSize: 4})
	i.newFile()
	d := i.active
	source := "/* 한글\n주석 */\n\tint main() { return 42; }\n"
	d.editor.SetText(source, false)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	d.syntax.SetRect(0, 0, 45, 8)
	colorAt := func(x, y int) tcell.Color {
		t.Helper()
		left, top, _, _ := d.syntax.GetInnerRect()
		_, _, style, _ := screen.GetContent(x+left, y+top)
		fg, _, _ := style.Decompose()
		return fg
	}
	d.syntax.Draw(screen)
	left, top, _, _ := d.syntax.GetInnerRect()
	if number, _, _, _ := screen.GetContent(left-2, top); number != '1' {
		t.Fatalf("first line number = %q", number)
	}
	if colorAt(4, 2) != foreground {
		t.Fatal("new document should use the default text color")
	}
	path := filepath.Join(root, "한글.c")
	i.save(d, path, nil)
	d.syntax.Draw(screen)
	if colorAt(0, 1) == foreground || colorAt(4, 2) == foreground || colorAt(4, 2) == colorAt(0, 1) {
		t.Fatal("multiline comment and tab-indented type must have distinct colors after Save As")
	}
	start := strings.Index(source, "42")
	d.editor.Select(start, start+2)
	d.syntax.Draw(screen)
	if colorAt(24, 2) != tcell.ColorWhite {
		t.Fatal("syntax color covered the selection")
	}
	d.editor.Replace(start, start+2, "\"한abc글\"")
	d.syntax.Draw(screen)
	if colorAt(25, 2) == foreground || colorAt(25, 2) == colorAt(4, 2) {
		t.Fatal("Korean string not recolored after edit")
	}
	d.syntax.SetOffset(2, 4)
	d.syntax.Draw(screen)
	if number, _, _, _ := screen.GetContent(left-2, top); number != '3' {
		t.Fatalf("line number after scrolling = %q", number)
	}
	if colorAt(0, 0) != syntaxColors[chroma.KeywordType] {
		t.Fatal("highlight shifted after scrolling")
	}
	i.save(d, filepath.Join(root, "plain.unknownformat"), nil)
	d.syntax.Draw(screen)
	if colorAt(0, 0) != foreground {
		t.Fatal("unknown extension retained stale colors")
	}
	data, err := os.ReadFile(d.disk.path)
	if err != nil || string(data) != strings.Replace(source, "42", "\"한abc글\"", 1) {
		t.Fatal("highlighting altered saved bytes", err)
	}
}

func TestCompletionPopupFollowsCursor(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	i.newFile()
	d := i.active
	d.disk.path = "main.go"
	source := "\t한글.pr\n\n\n\n\t한글.pr"
	d.editor.SetText(source, false)
	d.editor.SetRect(10, 4, 70, 7)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(90, 20)

	selected := -1
	i.acceptHint = func(index int) { selected = index }
	i.hints.AddItem("Print", "", 0, nil).AddItem("Printf", "", 0, nil)
	for _, where := range []struct {
		offset int
		above  bool
	}{{strings.IndexByte(source, '\n'), false}, {len(source), true}} {
		d.editor.Select(where.offset, where.offset)
		d.editor.Draw(screen)
		i.drawHints(screen)
		x, y, _, h := i.hints.GetRect()
		_, _, row, col := d.editor.GetCursor()
		rowOffset, colOffset := d.editor.GetOffset()
		cursorY := 5 + row - rowOffset
		if x != 11+col-colOffset || h != 4 || (!where.above && y != cursorY+1) || (where.above && y+h != cursorY) {
			t.Fatalf("popup is not adjacent to the visible cursor: rect origin (%d, %d), height %d, cursor row %d", x, y, h, cursorY)
		}
	}
	i.handleKey(tcell.NewEventKey(tcell.KeyDown, 0, tcell.ModNone))
	i.handleKey(tcell.NewEventKey(tcell.KeyTab, 0, tcell.ModNone))
	if selected != 1 || !d.editor.HasFocus() {
		t.Fatal("selection should apply the second result while editor retains focus")
	}
	if page, _ := i.dock.GetFrontPage(); page != "문제" {
		t.Fatalf("completion replaced dock page %q", page)
	}
}

func TestCompletionAcceptsMouseClick(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	i.newFile()
	screen := tcell.NewSimulationScreen("UTF-8")
	i.app.SetScreen(screen)
	screen.SetSize(100, 28)
	done := make(chan error, 1)
	go func() { done <- i.app.Run() }()
	t.Cleanup(func() {
		i.app.Stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("IDE failed to stop")
		}
	})

	selected := make(chan int, 1)
	i.app.QueueUpdateDraw(func() {
		i.acceptHint = func(index int) {
			i.clearHints()
			if !i.active.editor.HasFocus() {
				t.Error("editor lost focus while applying the completion")
			}
			selected <- index
		}
		i.hints.AddItem("Print", "", 0, nil).AddItem("Printf", "", 0, func() { i.acceptHint(1) })
	})
	var x, y int
	i.app.QueueUpdate(func() {
		x, y, _, _ = i.hints.GetInnerRect()
	})
	screen.InjectMouse(x, y+1, tcell.Button1, tcell.ModNone)
	screen.InjectMouse(x, y+1, tcell.ButtonNone, tcell.ModNone)
	select {
	case index := <-selected:
		if index != 1 {
			t.Fatalf("applied candidate %d, want 1", index)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("press and release did not apply a completion")
	}
}

func TestAssistantKeepsCodeVisible(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	i.newFile()
	i.active.editor.SetText("const keep = 1", false)
	i.showAssistant()
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	draw := func(width int) {
		screen.SetSize(width, 36)
		i.pages.SetRect(0, 0, width, 36)
		i.app.GetBeforeDrawFunc()(screen)
		i.pages.Draw(screen)
	}
	for _, width := range []int{145, 110, 88} {
		draw(width)
		x, y, w, h := i.active.editor.GetRect()
		ax, ay, aw, ah := i.assistant.GetRect()
		fx, _, fw, _ := i.tree.GetRect()
		if !i.treeShown || fw < 20 || fx+fw > x {
			t.Fatalf("file tree is missing from the split at %d columns", width)
		}
		if min(w, aw) < 40 || min(h, ah) < 8 || (width >= 120 && x+w > ax) || (width < 120 && y+h > ay) {
			t.Fatalf("overlapping or cramped editor/AI at %d columns: editor %d,%d %dx%d, AI %d,%d %dx%d", width, x, y, w, h, ax, ay, aw, ah)
		}
	}
	i.toggleTree()
	draw(145)
	if i.treeShown || !i.assistantOpen || !i.active.editor.HasFocus() {
		t.Fatal("closing the tree should keep AI visible and focus the editor")
	}
	i.toggleTree()
	draw(145)
	if !i.treeShown || !i.tree.HasFocus() {
		t.Fatal("opening the tree should focus the file list")
	}
	i.app.SetFocus(i.assistant)
	swap := tcell.NewEventKey(tcell.KeyCtrlT, 0, tcell.ModNone)
	i.handleKey(swap)
	if !i.active.editor.HasFocus() || !i.assistantOpen || i.active.editor.GetText() != "const keep = 1" {
		t.Fatal("switching to the editor should keep both panes and the document")
	}
	i.handleKey(swap)
	if !i.assistant.HasFocus() {
		t.Fatal("second switch should return focus to AI")
	}
}

func TestHorizontalScrollKeepsCaretAndSelection(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	i.newFile()
	d := i.active
	start := "\t한글"
	d.editor.SetText(start+strings.Repeat("x", 90), false)
	d.editor.SetRect(4, 2, 42, 8)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	d.syntax.Draw(screen)
	d.editor.Select(0, len(start))
	_, _, editorWidth, _ := d.syntax.GetInnerRect()
	step := max(4, editorWidth/2)
	check := func(want int) {
		t.Helper()
		row, col := d.editor.GetOffset()
		_, a, b := d.editor.GetSelection()
		if row != 0 || col != want || a != 0 || b != len(start) {
			t.Fatalf("scroll = %d,%d, selection = %d:%d; want column %d and the original selection", row, col, a, b, want)
		}
	}
	x, y, width := d.syntax.columnBarRect()
	handle := d.syntax.MouseHandler()
	mouse := func(action tview.MouseAction, col, row int, mod tcell.ModMask) {
		t.Helper()
		if ok, _ := handle(action, tcell.NewEventMouse(col, row, tcell.ButtonNone, mod), func(p tview.Primitive) { i.app.SetFocus(p) }); !ok {
			t.Fatal("scroll event was not consumed", action)
		}
	}
	mouse(tview.MouseLeftDown, x+width-1, y, tcell.ModNone)
	check(step)
	mouse(tview.MouseScrollDown, x+10, y-2, tcell.ModShift)
	check(step + 4)
	mouse(tview.MouseLeftDown, x+14, y, tcell.ModNone)
	mouse(tview.MouseMove, x+22, y, tcell.ModNone)
	mouse(tview.MouseLeftUp, x+22, y, tcell.ModNone)
	check(step + 12)
	i.handleKey(tcell.NewEventKey(tcell.KeyLeft, 0, tcell.ModAlt))
	check(12)
	for range 8 {
		mouse(tview.MouseLeftDown, x+width-1, y, tcell.ModNone)
	}
	check(99 - editorWidth) // Four tab cells, four Hangul cells, 90 ASCII cells, and room for the end caret.
}

func TestSelectedTabStaysInsideEditor(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4})
	i.newFile()
	editor := i.active.syntax
	editor.SetRect(20, 2, 50, 6)
	editor.SetText("\t"+strings.Repeat("x", 70), false)
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatal(err)
	}
	defer screen.Fini()
	screen.SetSize(80, 12)
	editor.Draw(screen)
	editor.Select(0, 15)
	editor.SetOffset(0, 10)
	x, y, _, _ := editor.GetInnerRect()
	for col := x - 10; col < 20; col++ {
		screen.SetContent(col, y, '!', nil, tcell.StyleDefault)
	}
	editor.Draw(screen)
	for col := x - 10; col < 20; col++ {
		if r, _, _, _ := screen.GetContent(col, y); r != '!' {
			t.Fatalf("selected tab overwrote adjacent cell %d: %q", col, r)
		}
	}
}

func TestEditorWorkflow(t *testing.T) {
	root := t.TempDir()
	i := newIDE(root, config{IndentSize: 4})
	i.newFile()
	screen := tcell.NewSimulationScreen("UTF-8")
	i.app.SetScreen(screen)
	screen.SetSize(120, 36)
	done := make(chan error, 1)
	go func() { done <- i.app.Run() }()
	t.Cleanup(func() {
		i.app.Stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(3 * time.Second):
			t.Error("IDE failed to stop")
		}
	})
	check := func(predicate func() bool) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			result := make(chan bool, 1)
			go i.app.QueueUpdateDraw(func() { result <- predicate() })
			select {
			case ok := <-result:
				if ok {
					return
				}
			case <-deadline:
				t.Fatal("IDE did not reach expected state")
			}
			select {
			case <-deadline:
				t.Fatal("IDE did not reach expected state")
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	key := func(key tcell.Key) { screen.InjectKey(key, 0, tcell.ModNone) }
	typeText := func(text string) {
		for _, r := range text {
			screen.InjectKey(tcell.KeyRune, r, tcell.ModNone)
		}
	}
	check(func() bool { return i.active != nil && i.treeShown })
	width, _ := screen.Size()
	screen.InjectMouse(width-25, 0, tcell.Button1, tcell.ModNone)
	screen.InjectMouse(width-25, 0, tcell.ButtonNone, tcell.ModNone)
	check(func() bool { return i.dialog })
	i.app.QueueUpdateDraw(func() {
		x, y, _, _ := i.app.GetFocus().GetRect()
		i.pages.MouseHandler()(tview.MouseLeftDown, tcell.NewEventMouse(x+2, y+2, tcell.Button1, tcell.ModNone), func(p tview.Primitive) { i.app.SetFocus(p) })
		if i.active.editor.HasFocus() {
			t.Error("menu press leaked through to the editor")
		}
	})
	key(tcell.KeyEscape)
	check(func() bool { return !i.dialog && i.app.GetFocus() == i.active.editor })
	key(tcell.KeyCtrlB)
	check(func() bool { return i.app.GetFocus() == i.tree })
	key(tcell.KeyCtrlB)
	check(func() bool { return i.app.GetFocus() == i.active.editor })
	key(tcell.KeyCtrlR)
	check(func() bool { return i.dialog })
	key(tcell.KeyEscape)
	check(func() bool { return !i.dialog })
	key(tcell.KeyCtrlE)
	check(func() bool { return i.dialog })
	key(tcell.KeyEscape)
	check(func() bool { return !i.dialog })
	key(tcell.KeyCtrlO)
	check(func() bool { return i.dialog })
	typeText("한글.txt")
	key(tcell.KeyEnter)
	check(func() bool { return !i.dialog && i.active.disk.path == filepath.Join(root, "한글.txt") })
	typeText("안녕하세요")
	key(tcell.KeyTab)
	typeText("code")
	check(func() bool { return i.active.dirty() && i.active.editor.GetText() == "안녕하세요  code" })
	key(tcell.KeyCtrlS)
	check(func() bool {
		data, _ := os.ReadFile(filepath.Join(root, "한글.txt"))
		return !i.active.dirty() && string(data) == "안녕하세요  code"
	})
	key(tcell.KeyCtrlA)
	key(tcell.KeyCtrlC)
	key(tcell.KeyCtrlN)
	key(tcell.KeyCtrlV)
	check(func() bool { return i.active.disk.path == "" && i.active.editor.GetText() == "안녕하세요  code" })
	key(tcell.KeyCtrlF)
	check(func() bool { return i.dialog })
	typeText("안녕")
	key(tcell.KeyEnter)
	check(func() bool { text, _, _ := i.active.editor.GetSelection(); return !i.dialog && text == "안녕" })
	key(tcell.KeyCtrlW)
	check(func() bool { return i.dialog })
	key(tcell.KeyCtrlC)
	check(func() bool { return !i.dialog && len(i.documents) == 3 })
	key(tcell.KeyCtrlS)
	check(func() bool { return i.dialog })
	typeText("copy.txt")
	key(tcell.KeyEnter)
	check(func() bool {
		data, _ := os.ReadFile(filepath.Join(root, "copy.txt"))
		return string(data) == "안녕하세요  code"
	})
	if err := os.WriteFile(filepath.Join(root, "copy.txt"), []byte("external"), 0644); err != nil {
		t.Fatal(err)
	}
	typeText("!")
	key(tcell.KeyCtrlS)
	check(func() bool { return i.dialog })
	key(tcell.KeyEscape)
	check(func() bool {
		data, _ := os.ReadFile(filepath.Join(root, "copy.txt"))
		return !i.dialog && string(data) == "external" && i.active.dirty()
	})
	key(tcell.KeyF1)
	check(func() bool { return i.dialog })
	screen.SetSize(60, 18)
	screen.PostEvent(tcell.NewEventResize(60, 18))
	check(func() bool { return !i.treeShown && i.dialog })
	key(tcell.KeyEscape)
	check(func() bool { return !i.dialog && i.app.GetFocus() == i.active.editor })
	i.app.QueueUpdateDraw(func() {
		i.addDocument(&document{path: filepath.Join(root, "large.py"), text: strings.Repeat("# before\n", 700)})
	})
	i.app.QueueUpdateDraw(func() {
		i.active.editor.Replace(0, len("# before"), "return 42")
	})
	check(func() bool {
		v := i.active.syntax
		return v.valid && len(v.spans) > 0 && v.spans[0].color == syntaxColors[chroma.Keyword]
	})
}

func TestTaskExecutionReturnsAndReloadsFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "external.txt")
	if err := os.WriteFile(path, []byte("before"), 0644); err != nil {
		t.Fatal(err)
	}
	i := newIDE(root, config{IndentSize: 4})
	if err := i.openFile(path); err != nil {
		t.Fatal(err)
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	i.app.SetScreen(screen)
	defer screen.Fini()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GALAXY_IDE_TEST_CHILD", "1")
	i.runTask(task{Name: "프로젝트 셸", Args: []string{executable, "-test.run=^TestTaskChild$", "--", path}})
	if i.active.editor.GetText() != "외부 도구가 수정한 내용\n" || i.active.dirty() {
		t.Fatal("task failed to return or reload a clean file", i.message)
	}
}

func TestProjectBuildOverridesCompilerAndChecksOtherTabs(t *testing.T) {
	i := newIDE(t.TempDir(), config{IndentSize: 4, Tasks: []task{
		{Name: "검사", Group: "test", Args: []string{"make", "test"}},
		{Name: "데모", Group: "build", Args: []string{"make", "demo"}},
	}})
	i.addDocument(&document{path: filepath.Join(i.root, "include.h")})
	i.active.editor.SetText("change", false)
	i.addDocument(&document{path: filepath.Join(i.root, "main.c")})
	build, ok := i.defaultBuild()
	if !ok || build.Group != "build" || build.Args[1] != "demo" {
		t.Fatal(build)
	}
	i.handleKey(tcell.NewEventKey(tcell.KeyCtrlR, 0, tcell.ModNone))
	if !i.dialog {
		t.Fatal("build did not prompt for the dirty header")
	}
	i.dismiss()
	i.newFile()
	build, ok = i.defaultBuild()
	if !ok || build.Group != "build" {
		t.Fatal("project build requires an active source")
	}
	i.config.Tasks = nil
	if _, ok := i.defaultBuild(); ok {
		t.Fatal("blank tab should use the task menu")
	}
	i.activate(i.documents[1])
	if fallback, ok := i.defaultBuild(); !ok || !fallback.buildAndRun {
		t.Fatal("missing native fallback")
	}
}

func TestTaskChild(t *testing.T) {
	if os.Getenv("GALAXY_IDE_TEST_CHILD") != "1" {
		return
	}
	if err := os.WriteFile(os.Args[len(os.Args)-1], []byte("외부 도구가 수정한 내용\n"), 0644); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
