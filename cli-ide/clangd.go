package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

type rpcParams = map[string]any
type lspPos struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}
type lspRange struct {
	Start lspPos `json:"start"`
	End   lspPos `json:"end"`
}
type completion struct {
	Label, InsertText, FilterText, Detail string
	InsertTextFormat                      int
	TextEdit                              *struct {
		Range   *lspRange
		Replace *lspRange
		NewText string
	}
}
type location struct {
	URI                  string
	Range                lspRange
	TargetURI            string
	TargetSelectionRange lspRange
}

func fileURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return (&url.URL{Scheme: "file", Path: path}).String()
}
func uriPath(s string) (string, error) {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "file" || u.Host != "" && u.Host != "localhost" {
		return "", errors.New("로컬 파일 위치를 확인할 수 없습니다")
	}
	p := u.Path
	if filepath.Separator == '\\' && len(p) >= 3 && p[0] == '/' && p[2] == ':' {
		p = strings.ToUpper(p[1:2]) + p[2:]
	}
	p = filepath.FromSlash(p)
	if !filepath.IsAbs(p) {
		return "", errors.New("정의 경로가 올바르지 않습니다")
	}
	return p, nil
}
func lspPosition(s string, offset int) lspPos {
	prefix := s[:offset]
	line := prefix[strings.LastIndexByte(prefix, '\n')+1:]
	n := 0
	for _, r := range line {
		n += utf16.RuneLen(r)
	}
	return lspPos{strings.Count(prefix, "\n"), n}
}
func bytePosition(s string, p lspPos) (int, bool) {
	if p.Line < 0 || p.Character < 0 {
		return 0, false
	}
	off := 0
	for n := 0; n < p.Line; n++ {
		k := strings.IndexByte(s[off:], '\n')
		if k < 0 {
			return 0, false
		}
		off += k + 1
	}
	units := 0
	for k, r := range s[off:] {
		if units == p.Character {
			return off + k, true
		}
		if r == '\n' || units > p.Character {
			return 0, false
		}
		units += utf16.RuneLen(r)
	}
	return len(s), units == p.Character
}

type languageServer struct {
	name                         string
	stdout                       io.ReadCloser
	responses                    chan rpcMessage
	done                         chan struct{}
	writeMu                      sync.Mutex
	documents                    sync.Map
	reports                      sync.Map
	onReport                     func(languageReport)
	incremental, pullDiagnostics bool
	cmd                          *exec.Cmd
	in                           io.WriteCloser
	out                          *bufio.Reader
	cacheDir                     string
	id, version                  int
	open                         map[string]string
}

// Give plain source folders an index without writing build files into the project.
func compilationDir(p nativeProject) (string, error) {
	for dir := p.root; ; dir = filepath.Dir(dir) {
		for _, name := range []string{"compile_commands.json", "compile_flags.txt", ".clangd", "build/compile_commands.json"} {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return "", nil
			}
		}
		if dir == filepath.Dir(dir) {
			break
		}
	}
	home, _ := os.UserHomeDir()
	if p.root == home || p.root == filepath.Dir(p.root) {
		return "", nil
	}
	var entries []rpcParams
	err := p.walkSources(func(f string) {
		cc := "clang"
		if nativeLanguage(f) == "c++" {
			cc = "clang++"
		}
		entries = append(entries, rpcParams{"directory": p.root, "file": f, "arguments": []string{cc, "-I", p.root, "-I", filepath.Join(p.root, "include"), "-c", f}})
	})
	if err != nil || len(entries) == 0 {
		return "", err
	}
	dir, err := os.MkdirTemp("", "galaxy-clangd-*")
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(entries)
	if err == nil {
		err = os.WriteFile(filepath.Join(dir, "compile_commands.json"), data, 0600)
	}
	if err != nil {
		os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func startLanguageServer(ctx context.Context, p nativeProject, tools languageTools, onReport func(languageReport)) (*languageServer, error) {
	bin, err := exec.LookPath(tools.server)
	if err != nil {
		return nil, fmt.Errorf("%s 설치 필요 · IDE README의 언어별 설치 방법을 확인하세요", tools.server)
	}
	args := tools.args
	dir := ""
	switch tools.server {
	case "clangd":
		dir, err = compilationDir(p)
		if dir != "" {
			args = append(args, "--compile-commands-dir="+dir)
		}
	case "jdtls":
		dir, err = os.MkdirTemp("", "galaxy-jdtls-*")
		args = append(args, "--jvm-arg=-Xms64m", "--jvm-arg=-Xmx768m", "-data", dir)
	}
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	isolateLanguageServer(cmd)
	cmd.Dir = p.root
	in, err := cmd.StdinPipe()
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err == nil {
		err = cmd.Start()
	}
	if err != nil {
		in.Close()
		os.RemoveAll(dir)
		return nil, err
	}
	s := &languageServer{cmd: cmd, in: in, stdout: out, out: bufio.NewReader(out), cacheDir: dir, open: map[string]string{}, name: tools.server, responses: make(chan rpcMessage, 1), done: make(chan struct{}), onReport: onReport}
	go s.readLoop()
	return s, nil
}
func (s *languageServer) close() {
	timeout := time.AfterFunc(2*time.Second, func() { s.cmd.Process.Kill(); s.stdout.Close(); s.in.Close() })
	if _, err := s.call("shutdown", nil); err == nil {
		_ = s.notify("exit", nil)
	}
	close(s.done)
	s.in.Close()
	s.cmd.Wait()
	timeout.Stop()
	if s.cacheDir != "" {
		os.RemoveAll(s.cacheDir)
	}
}
func (s *languageServer) write(msg rpcParams) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	msg["jsonrpc"] = "2.0"
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err = fmt.Fprintf(s.in, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return err
	}
	_, err = s.in.Write(b)
	return err
}
func (s *languageServer) notify(method string, params any) error {
	return s.write(rpcParams{"method": method, "params": params})
}
func (s *languageServer) call(method string, params any) (json.RawMessage, error) {
	s.id++
	if err := s.write(rpcParams{"method": method, "params": params, "id": s.id}); err != nil {
		return nil, err
	}
	for msg := range s.responses {
		if string(msg.ID) != strconv.Itoa(s.id) {
			continue
		}
		if msg.Error != nil {
			return nil, errors.New(msg.Error.Message)
		}
		return msg.Result, nil
	}
	return nil, fmt.Errorf("%s 연결이 종료됐습니다", s.name)
}
func (s *languageServer) syncFiles(files map[string]string) error {
	for f := range s.open {
		if _, ok := files[f]; !ok {
			if err := s.notify("textDocument/didClose", rpcParams{"textDocument": rpcParams{"uri": fileURI(f)}}); err != nil {
				return err
			}
			s.documents.Delete(f)
			s.reports.Delete(f)
			delete(s.open, f)
		}
	}
	for f, text := range files {
		old, ok := s.open[f]
		if ok && old == text {
			continue
		}
		s.version++
		doc := rpcParams{"uri": fileURI(f), "version": s.version}
		s.documents.Store(f, serverDocument{text, s.version})
		s.reports.Delete(f)
		var err error
		if !ok {
			doc["languageId"] = toolsFor(f).id
			doc["text"] = text
			err = s.notify("textDocument/didOpen", rpcParams{"textDocument": doc})
		} else {
			change := rpcParams{"text": text}
			if s.incremental {
				change["range"] = lspRange{End: lspPosition(old, len(old))}
			}
			err = s.notify("textDocument/didChange", rpcParams{"textDocument": doc, "contentChanges": []rpcParams{change}})
		}
		if err != nil {
			return err
		}
		s.open[f] = text
	}
	return nil
}

type languageClient struct {
	onReport func(languageReport)
	mu       sync.Mutex
	server   *languageServer
}

func (c *languageClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.server != nil {
		c.server.close()
		c.server = nil
	}
}
func (c *languageClient) query(life, request context.Context, p nativeProject, files map[string]string, path string, pos lspPos, method string) (data json.RawMessage, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if request.Err() != nil {
		return nil, request.Err()
	}
	tools := toolsFor(path)
	if tools.server == "" {
		return nil, errors.New("지원하지 않는 언어입니다")
	}
	if c.server != nil && (c.server.cmd.Dir != p.root || c.server.name != tools.server) {
		c.server.close()
		c.server = nil
	}
	fresh := c.server == nil
	if fresh {
		c.server, err = startLanguageServer(life, p, tools, c.onReport)
		if err != nil {
			return nil, err
		}
	}
	s := c.server
	timeout := 30 * time.Second
	if fresh && tools.server == "jdtls" {
		timeout = 90 * time.Second
	}
	timer := time.AfterFunc(timeout, func() { s.cmd.Process.Kill(); s.stdout.Close() })
	defer func() {
		timer.Stop()
		if err != nil {
			s.close()
			c.server = nil
		}
	}()
	if fresh {
		if err = s.initialize(p.root); err != nil {
			return nil, err
		}
		if err = s.notify("initialized", rpcParams{}); err != nil {
			return nil, err
		}
		if s.name == "pyright-langserver" {
			if err = s.notify("workspace/didChangeConfiguration", rpcParams{"settings": rpcParams{}}); err != nil {
				return nil, err
			}
		}
	}
	if err = s.syncFiles(files); err != nil {
		return nil, err
	}
	if method == "diagnostic" {
		return s.diagnostics(path)
	}
	return s.call("textDocument/"+method, rpcParams{"textDocument": rpcParams{"uri": fileURI(path)}, "position": pos})
}
