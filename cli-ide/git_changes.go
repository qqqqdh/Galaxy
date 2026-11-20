package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/rivo/tview"
)

// Keep draining the pipe so Git can exit normally even for a large patch.
type gitOutput struct {
	data      bytes.Buffer
	truncated bool
}

func (b *gitOutput) Write(p []byte) (int, error) {
	n := min(len(p), (1<<20)-b.data.Len())
	b.data.Write(p[:n])
	b.truncated = b.truncated || n < len(p)
	return len(p), nil
}

func readGit(ctx context.Context, dir string, noIndex bool, args ...string) (string, bool, error) {
	options := []string{"--no-pager", "--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "core.quotePath=false"}
	cmd := exec.CommandContext(ctx, "git", append(options, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stdout, stderr gitOutput
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	var exit *exec.ExitError
	if err != nil && !(noIndex && errors.As(err, &exit) && exit.ExitCode() == 1 && (stdout.data.Len() > 0 || stderr.data.Len() == 0)) {
		switch {
		case errors.Is(err, exec.ErrNotFound):
			return "", false, errors.New("Ubuntu에서 git 명령을 찾을 수 없습니다")
		case strings.Contains(stderr.data.String(), "not a git repository"):
			return "", false, errors.New("Git 저장소가 아닙니다")
		case stderr.data.Len() > 0:
			return "", false, errors.New(strings.TrimSpace(stderr.data.String()))
		default:
			return "", false, err
		}
	}
	return stdout.data.String(), stdout.truncated, nil
}

type gitChange struct {
	path, previous, stage string
	code                  byte
}

func (c gitChange) label() string {
	name := strings.NewReplacer("\n", "\\n", "\r", "\\r", "\t", "\\t")
	path := name.Replace(c.path)
	if c.previous != "" {
		path = name.Replace(c.previous) + " → " + path
	}
	return fmt.Sprintf("%s  %c  %s", c.stage, c.code, path)
}

func gitChanges(ctx context.Context, dir string) (string, []gitChange, error) {
	top, _, err := readGit(ctx, dir, false, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil, err
	}
	root := filepath.FromSlash(strings.TrimSuffix(top, "\n"))
	data, limited, err := readGit(ctx, root, false, "status", "--porcelain=v1", "-z", "--find-renames", "--untracked-files=all", "--ignore-submodules=none")
	if err != nil {
		return "", nil, err
	}
	var out []gitChange
	for data != "" && len(out) < 2000 {
		line, rest, ok := strings.Cut(data, "\x00")
		if !ok || len(line) < 4 {
			break
		}
		data = rest
		x, y, path := line[0], line[1], line[3:]
		var prev string
		if strings.ContainsAny(line[:2], "RC") {
			prev, data, ok = strings.Cut(data, "\x00")
			if !ok {
				break
			}
		}
		switch {
		case x == '?':
			out = append(out, gitChange{path: path, stage: "새 파일", code: '?'})
		case x == 'U' || y == 'U' || line[:2] == "DD" || line[:2] == "AA":
			out = append(out, gitChange{path: path, stage: "충돌", code: 'U'})
		default:
			for index, code := range []byte{x, y} {
				if code == ' ' {
					continue
				}
				c := gitChange{path: path, stage: "작업 트리", code: code}
				if index == 0 {
					c.stage = "스테이징"
				}
				if code == 'R' || code == 'C' {
					c.previous = prev
				}
				out = append(out, c)
			}
		}
	}
	if limited || data != "" {
		return root, out, errors.New("목록 표시 한도에 도달했습니다")
	}
	return root, out, nil
}

func (c gitChange) diff(ctx context.Context, root string) (string, error) {
	args := []string{"diff", "--no-color", "--no-ext-diff", "--no-textconv", "--submodule=short", "--unified=3"}
	untracked := c.code == '?'
	switch {
	case untracked:
		info, err := os.Lstat(filepath.Join(root, c.path))
		if err != nil {
			return "", err
		}
		if !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			return "", errors.New("디렉터리 또는 특수 파일입니다")
		}
		args = append(args, "--no-index", "--", "/dev/null", c.path)
	default:
		if c.stage == "스테이징" {
			args = append(args, "--cached")
		}
		args = append(args, "--find-renames", "--", c.path)
		if c.previous != "" {
			args = append(args, c.previous)
		}
	}
	patch, limited, err := readGit(ctx, root, untracked, args...)
	if err != nil {
		return "", err
	}
	if patch == "" {
		return "줄 단위 변경 없음 · 변경사항 탭을 다시 누르면 목록을 갱신합니다", nil
	}
	var view strings.Builder
	hunk := false
	for line := range strings.SplitSeq(strings.TrimSuffix(patch, "\n"), "\n") {
		color := ""
		switch {
		case strings.HasPrefix(line, "diff "):
			hunk = false
		case strings.HasPrefix(line, "@@"):
			hunk = true
			color = "[#90caf9]"
		case hunk && strings.HasPrefix(line, "+"):
			color = "[#80dca0]"
		case hunk && strings.HasPrefix(line, "-"):
			color = "[#ff9292]"
		}
		view.WriteString(color)
		view.WriteString(tview.Escape(line))
		if color != "" {
			view.WriteString("[-]")
		}
		view.WriteByte('\n')
	}
	if limited {
		view.WriteString("… 처음 1 MiB까지 표시")
	}
	return view.String(), nil
}

func (i *ide) refreshChanges() {
	if i.changesCancel != nil {
		i.changesCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	i.changesCancel = cancel
	i.changeList.Clear().AddItem("Git 변경사항 읽는 중…", "", 0, nil)
	go func() {
		deadline, done := context.WithTimeout(ctx, 30*time.Second)
		defer done()
		root, changes, err := gitChanges(deadline, i.root)
		if ctx.Err() != nil {
			return
		}
		i.app.QueueUpdateDraw(func() {
			if ctx.Err() != nil {
				return
			}
			i.changeList.Clear()
			for _, c := range changes {
				i.changeList.AddItem(tview.Escape(c.label()), "", 0, func() { i.showDiff(root, c) })
			}
			if err != nil {
				i.changeList.AddItem(tview.Escape(err.Error()), "", 0, nil)
			} else if len(changes) == 0 {
				i.changeList.AddItem("변경된 파일 없음", "", 0, nil)
			}
		})
	}()
}

func (i *ide) showDiff(root string, c gitChange) {
	view := tview.NewTextView().SetDynamicColors(true).SetWrap(false).SetText("비교 중…")
	box := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(view, 0, 1, true).
		AddItem(tview.NewButton("목록으로 · Esc").SetSelectedFunc(i.dismiss), 1, 0, false)
	box.SetBorder(true).SetTitle(" " + tview.Escape(c.label()) + " ")
	ctx, cancel := context.WithCancel(context.Background())
	i.diffCancel = cancel
	i.showDialog(box, 116, 34)
	go func() {
		deadline, done := context.WithTimeout(ctx, 30*time.Second)
		defer done()
		patch, err := c.diff(deadline, root)
		if err != nil {
			patch = tview.Escape(err.Error())
		}
		if ctx.Err() != nil {
			return
		}
		i.app.QueueUpdateDraw(func() {
			if ctx.Err() == nil {
				view.SetText(patch).ScrollToBeginning()
			}
		})
	}()
}
