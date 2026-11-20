package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/textproto"
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
type lspRange struct{ Start, End lspPos }
type completion struct {
	Label, InsertText, FilterText, Detail string
	TextEdit                              *struct {
		Range   lspRange
		NewText string
	}
}
type location struct {
	URI   string
	Range lspRange
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
		p = p[1:]
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

type clangServer struct {
	cmd         *exec.Cmd
	in          io.WriteCloser
	out         *bufio.Reader
	cacheDir    string
	id, version int
	open        map[string]string
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

func startClang(ctx context.Context, p nativeProject) (*clangServer, error) {
	bin, err := exec.LookPath("clangd")
	if err != nil {
		return nil, errors.New("clangd 설치 필요 · sudo apt install clangd")
	}
	dir, err := compilationDir(p)
	if err != nil {
		return nil, err
	}
	args := []string{"-j=2", "--background-index", "--header-insertion=never", "--limit-results=40", "--log=error"}
	if dir != "" {
		args = append(args, "--compile-commands-dir="+dir)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
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
	return &clangServer{cmd: cmd, in: in, out: bufio.NewReader(out), cacheDir: dir, open: map[string]string{}}, nil
}
func (s *clangServer) close() {
	s.in.Close()
	s.cmd.Process.Kill()
	s.cmd.Wait()
	if s.cacheDir != "" {
		os.RemoveAll(s.cacheDir)
	}
}
func (s *clangServer) write(msg rpcParams) error {
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
func (s *clangServer) notify(method string, params any) error {
	return s.write(rpcParams{"method": method, "params": params})
}
func (s *clangServer) call(method string, params any) (json.RawMessage, error) {
	s.id++
	if err := s.write(rpcParams{"method": method, "params": params, "id": s.id}); err != nil {
		return nil, err
	}
	for {
		h, err := textproto.NewReader(s.out).ReadMIMEHeader()
		if err != nil {
			return nil, err
		}
		n, err := strconv.Atoi(h.Get("Content-Length"))
		if err != nil || n < 0 || n > 32<<20 {
			return nil, errors.New("clangd 응답 길이 오류")
		}
		buf := make([]byte, n)
		if _, err = io.ReadFull(s.out, buf); err != nil {
			return nil, err
		}
		var msg struct {
			ID     json.RawMessage
			Method string
			Result json.RawMessage
			Error  *struct{ Message string }
		}
		if err = json.Unmarshal(buf, &msg); err != nil {
			return nil, err
		}
		if msg.Method != "" && len(msg.ID) > 0 {
			if err = s.write(rpcParams{"id": msg.ID, "error": rpcParams{"code": -32601, "message": "Unsupported client method"}}); err != nil {
				return nil, err
			}
			continue
		}
		if string(msg.ID) != strconv.Itoa(s.id) {
			continue
		}
		if msg.Error != nil {
			return nil, errors.New(msg.Error.Message)
		}
		return msg.Result, nil
	}
}

func clangLanguage(path string) string {
	if nativeLanguage(path) == "c" {
		return "c"
	}
	if nativeLanguage(path) == "c++" {
		return "cpp"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".h", ".hh", ".hpp", ".hxx", ".ipp":
		return "cpp"
	}
	return ""
}
func (s *clangServer) syncFiles(files map[string]string) error {
	for f := range s.open {
		if _, ok := files[f]; !ok {
			if err := s.notify("textDocument/didClose", rpcParams{"textDocument": rpcParams{"uri": fileURI(f)}}); err != nil {
				return err
			}
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
		var err error
		if !ok {
			doc["languageId"] = clangLanguage(f)
			doc["text"] = text
			err = s.notify("textDocument/didOpen", rpcParams{"textDocument": doc})
		} else {
			err = s.notify("textDocument/didChange", rpcParams{"textDocument": doc, "contentChanges": []rpcParams{{"text": text}}})
		}
		if err != nil {
			return err
		}
		s.open[f] = text
	}
	return nil
}

type clangClient struct {
	mu     sync.Mutex
	server *clangServer
}

func (c *clangClient) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.server != nil {
		c.server.close()
		c.server = nil
	}
}
func (c *clangClient) query(life, request context.Context, p nativeProject, files map[string]string, path string, pos lspPos, method string) (data json.RawMessage, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if request.Err() != nil {
		return nil, request.Err()
	}
	if c.server != nil && c.server.cmd.Dir != p.root {
		c.server.close()
		c.server = nil
	}
	fresh := c.server == nil
	if fresh {
		c.server, err = startClang(life, p)
		if err != nil {
			return nil, err
		}
	}
	s := c.server
	timer := time.AfterFunc(30*time.Second, func() { s.cmd.Process.Kill() })
	defer func() {
		timer.Stop()
		if err != nil {
			s.close()
			c.server = nil
		}
	}()
	if fresh {
		_, err = s.call("initialize", rpcParams{"processId": os.Getpid(), "rootUri": fileURI(p.root), "capabilities": rpcParams{"textDocument": rpcParams{"completion": rpcParams{"completionItem": rpcParams{"snippetSupport": false}}}}})
		if err != nil {
			return nil, err
		}
		if err = s.notify("initialized", rpcParams{}); err != nil {
			return nil, err
		}
	}
	if err = s.syncFiles(files); err != nil {
		return nil, err
	}
	return s.call("textDocument/"+method, rpcParams{"textDocument": rpcParams{"uri": fileURI(path)}, "position": pos})
}
