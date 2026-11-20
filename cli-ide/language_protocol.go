package main

import (
	"encoding/json"
	"io"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type rpcMessage struct {
	ID, Result, Params json.RawMessage
	Method             string
	Error              *struct{ Message string }
}
type serverDocument struct {
	text    string
	version int
}
type languageDiagnostic struct {
	Range    lspRange
	Severity int
	Message  string
}
type languageReport struct {
	path, text string
	items      []languageDiagnostic
}

func (s *languageServer) initialize(root string) error {
	folders := []rpcParams{{"uri": fileURI(root), "name": filepath.Base(root)}}
	data, err := s.call("initialize", rpcParams{
		"processId": os.Getpid(), "rootUri": fileURI(root), "workspaceFolders": folders,
		"initializationOptions": rpcParams{"embeddedLanguages": rpcParams{"css": true, "javascript": true}},
		"capabilities": rpcParams{
			"workspace": rpcParams{"configuration": true, "workspaceFolders": true},
			"textDocument": rpcParams{"completion": rpcParams{"completionItem": rpcParams{"snippetSupport": s.name == "vscode-css-language-server" || s.name == "vscode-html-language-server"}},
				"publishDiagnostics": rpcParams{"versionSupport": true}, "diagnostic": rpcParams{}},
		},
	})
	if err != nil {
		return err
	}
	var info struct {
		Capabilities struct{ TextDocumentSync, DiagnosticProvider json.RawMessage }
	}
	if err = json.Unmarshal(data, &info); err != nil {
		return err
	}
	var mode int
	if json.Unmarshal(info.Capabilities.TextDocumentSync, &mode) != nil {
		var sync struct{ Change int }
		_ = json.Unmarshal(info.Capabilities.TextDocumentSync, &sync)
		mode = sync.Change
	}
	s.incremental = mode == 2
	provider := info.Capabilities.DiagnosticProvider
	s.pullDiagnostics = len(provider) > 0 && string(provider) != "null" && string(provider) != "false"
	return nil
}

func (s *languageServer) readLoop() {
	defer close(s.responses)
	reader := textproto.NewReader(s.out)
	for {
		header, err := reader.ReadMIMEHeader()
		if err != nil {
			return
		}
		n, err := strconv.Atoi(header.Get("Content-Length"))
		if err != nil || n <= 0 || n > 32<<20 {
			return
		}
		buf := make([]byte, n)
		if _, err = io.ReadFull(s.out, buf); err != nil {
			return
		}
		var msg rpcMessage
		if json.Unmarshal(buf, &msg) != nil {
			return
		}
		switch {
		case msg.Method == "textDocument/publishDiagnostics":
			var event struct {
				URI         string
				Version     *int
				Diagnostics []languageDiagnostic
			}
			if json.Unmarshal(msg.Params, &event) != nil {
				continue
			}
			path, err := uriPath(event.URI)
			if err != nil {
				continue
			}
			entry, ok := s.documents.Load(path)
			if !ok {
				continue
			}
			doc := entry.(serverDocument)
			if event.Version != nil && *event.Version != doc.version {
				continue
			}
			report := languageReport{path, doc.text, event.Diagnostics}
			s.reports.Store(path, report)
			if s.onReport != nil {
				s.onReport(report)
			}
		case msg.Method != "" && len(msg.ID) > 0:
			reply := rpcParams{"id": msg.ID, "result": nil}
			switch msg.Method {
			case "workspace/configuration":
				var query struct{ Items []json.RawMessage }
				_ = json.Unmarshal(msg.Params, &query)
				reply["result"] = make([]any, len(query.Items))
			case "workspace/workspaceFolders":
				reply["result"] = []rpcParams{{"uri": fileURI(s.cmd.Dir), "name": filepath.Base(s.cmd.Dir)}}
			case "window/workDoneProgress/create", "workspace/diagnostic/refresh":
			default:
				delete(reply, "result")
				reply["error"] = rpcParams{"code": -32601, "message": "Unsupported client method"}
			}
			if s.write(reply) != nil {
				return
			}
		case msg.Method == "" && len(msg.ID) > 0:
			select {
			case s.responses <- msg:
			case <-s.done:
				return
			}
		}
	}
}

func (s *languageServer) diagnostics(path string) (json.RawMessage, error) {
	if s.pullDiagnostics {
		data, err := s.call("textDocument/diagnostic", rpcParams{"textDocument": rpcParams{"uri": fileURI(path)}})
		if err != nil {
			return nil, err
		}
		var report struct{ Items []languageDiagnostic }
		if err = json.Unmarshal(data, &report); err != nil {
			return nil, err
		}
		return json.Marshal(report.Items)
	}
	if v, ok := s.reports.Load(path); ok && v.(languageReport).text == s.open[path] {
		return json.Marshal(v.(languageReport).items)
	}
	return nil, nil
}

func (r languageReport) problems() []problem {
	var out []problem
	lines := strings.Split(r.text, "\n")
	for _, d := range r.items {
		row := d.Range.Start.Line
		if row < 0 || row >= len(lines) {
			continue
		}
		start, ok := bytePosition(lines[row], lspPos{Character: d.Range.Start.Character})
		if !ok {
			continue
		}
		end := len(lines[row])
		if d.Range.End.Line == row {
			if pos, ok := bytePosition(lines[row], lspPos{Character: d.Range.End.Character}); ok {
				end = pos
			}
		}
		kind := "error"
		if d.Severity == 2 {
			kind = "warning"
		} else if d.Severity > 2 {
			kind = "note"
		}
		out = append(out, problem{file: r.path, kind: kind, message: d.Message, line: row + 1, first: start + 1, last: max(start+1, end)})
	}
	return out
}
