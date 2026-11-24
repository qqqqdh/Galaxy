package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const (
	logLimit = 2 * 1024 * 1024
	logTail  = 32 * 1024
)

type serverRun struct{ dir string }

func (r serverRun) path(name string) string { return filepath.Join(r.dir, name) }

func (r serverRun) running() (bool, error) {
	if _, err := os.Stat(r.dir); errors.Is(err, os.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	f, err := takeLock(r.path("running.lock"))
	if errors.Is(err, unix.EWOULDBLOCK) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return false, f.Close()
}

func (r serverRun) start(p profile) error {
	if err := os.MkdirAll(r.dir, 0700); err != nil {
		return err
	}
	f, err := takeLock(r.path("running.lock"))
	if errors.Is(err, unix.EWOULDBLOCK) {
		return errors.New("이미 실행 중입니다")
	}
	if err != nil {
		return err
	}
	defer f.Close()
	for _, name := range []string{"stop", "server.log", "server.log.1"} {
		if err := os.Remove(r.path(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := r.setResult("시작 중"); err != nil {
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.Command(binary, "--worker", r.dir, p.Dir, p.Command)
	// Keep the worker in this Ubuntu session while releasing the terminal UI.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.ExtraFiles = []*os.File{f}
	if err := cmd.Start(); err != nil {
		return errors.Join(err, r.setResult("시작 실패"))
	}
	// The child holds the same open file description. Closing f here keeps
	// the lock held until the worker closes its copy, including across UI exits.
	go func() { _ = cmd.Wait() }()
	return nil
}

func (r serverRun) requestStop() error {
	running, err := r.running()
	if err != nil {
		return err
	}
	if !running {
		return errors.New("실행 중인 서버가 없습니다")
	}
	return writeState(r.path("stop"), nil)
}

func (r serverRun) setResult(message string) error {
	return writeState(r.path("status"), []byte(message))
}

func (r serverRun) status() string {
	active, err := r.running()
	if err != nil {
		return err.Error()
	}
	if active {
		if _, err := os.Stat(r.path("stop")); err == nil {
			return "중지 중"
		}
	}
	result, _ := os.ReadFile(r.path("status"))
	switch {
	case active && string(result) == "실행 중":
		return "실행 중"
	case active:
		return "시작 중"
	case string(result) == "실행 중" || string(result) == "시작 중":
		return "실행 프로세스 종료됨"
	case len(result) > 0:
		return string(result)
	default:
		return "대기"
	}
}

func serve(r serverRun, dir, command string) error {
	lock := os.NewFile(3, "running.lock")
	defer lock.Close()
	if _, err := lock.Stat(); err != nil {
		return err
	}
	// Keep the lock in the worker only, even if the server spawns children.
	unix.CloseOnExec(3)
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer signal.Stop(stop)

	log := &rollingLog{path: r.path("server.log")}
	if err := log.open(); err != nil {
		return errors.Join(err, r.setResult("로그 열기 실패"))
	}
	defer log.close()

	cmd := exec.Command("/bin/sh", "-c", command)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = log, log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(log, err)
		return errors.Join(err, r.setResult("시작 실패"))
	}
	if err := r.setResult("실행 중"); err != nil {
		fmt.Fprintln(log, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	stopping, waitErr := waitForServer(r, stop, cmd.Process.Pid, done)
	if stopping {
		return r.setResult("중지됨")
	}
	if waitErr != nil {
		fmt.Fprintln(log, waitErr)
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
			return r.setResult(fmt.Sprintf("종료 · 코드 %d", cmd.ProcessState.ExitCode()))
		}
		return r.setResult("오류로 종료됨")
	}
	return r.setResult("종료 · 코드 0")
}

func waitForServer(r serverRun, stop <-chan os.Signal, pid int, done <-chan error) (bool, error) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	var deadline <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	stopping := false
	terminate := func() {
		if !stopping {
			stopping = true
			_ = unix.Kill(-pid, unix.SIGTERM)
			timer = time.NewTimer(4 * time.Second)
			deadline = timer.C
		}
	}
	for {
		select {
		case err := <-done:
			// A shell can exit while leaving children behind in its process group.
			if unix.Kill(-pid, 0) == nil {
				terminateChildren(pid, timer)
			}
			return stopping, err
		case <-stop:
			terminate()
		case <-tick.C:
			if _, err := os.Stat(r.path("stop")); err == nil {
				terminate()
			}
		case <-deadline:
			_ = unix.Kill(-pid, unix.SIGKILL)
			deadline = nil
		}
	}
}

func terminateChildren(pid int, timer *time.Timer) {
	if timer != nil {
		// The leader has exited. A further grace period for children is bounded.
		timer.Stop()
	}
	_ = unix.Kill(-pid, unix.SIGTERM)
	end := time.Now().Add(2 * time.Second)
	for time.Now().Before(end) {
		if unix.Kill(-pid, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = unix.Kill(-pid, unix.SIGKILL)
}

type rollingLog struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int
}

func (l *rollingLog) open() error {
	var err error
	l.file, err = os.OpenFile(l.path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	l.size = 0
	return err
}

func (l *rollingLog) close() {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

func (l *rollingLog) Write(data []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	written := 0
	for len(data) != 0 {
		if l.size == logLimit {
			l.close()
			if err := os.Rename(l.path, l.path+".1"); err != nil {
				return written, err
			}
			if err := l.open(); err != nil {
				return written, err
			}
		}
		if l.file == nil {
			return written, os.ErrClosed
		}
		n, err := l.file.Write(data[:min(len(data), logLimit-l.size)])
		l.size += n
		written += n
		if err != nil {
			return written, err
		}
		data = data[n:]
	}
	return written, nil
}

func (r serverRun) logs() string {
	current := tail(r.path("server.log"), logTail)
	older := tail(r.path("server.log.1"), logTail-len(current))
	return older + current
}

func tail(path string, maxBytes int) string {
	if maxBytes == 0 {
		return ""
	}
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return ""
	}
	if _, err := f.Seek(max(0, info.Size()-int64(maxBytes)), io.SeekStart); err != nil {
		return ""
	}
	data, _ := io.ReadAll(io.LimitReader(f, int64(maxBytes)))
	return string(data)
}
