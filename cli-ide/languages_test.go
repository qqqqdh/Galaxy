package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/chroma/v2/lexers"
)

func TestLanguageToolsAndActions(t *testing.T) {
	for _, tc := range []struct{ file, id, server, command string }{
		{"Demo.java", "java", "jdtls", "java"}, {"demo.cjs", "javascript", "typescript-language-server", "node"},
		{"index.htm", "html", "vscode-html-language-server", ""}, {"theme.css", "css", "vscode-css-language-server", ""},
		{"demo.go", "go", "gopls", "go"}, {"demo.py", "python", "pyright-langserver", "python3"}, {"demo.rs", "rust", "rust-analyzer", "sh"},
	} {
		tools := toolsFor(tc.file)
		task, ok := fileTask(tc.file)
		if !ok || task.preview != (tc.command == "") || tc.command != "" && (len(task.Args) == 0 || task.Args[0] != tc.command) ||
			tools.id != tc.id || tools.server != tc.server || lexers.Match(tc.file) == nil {
			t.Errorf("%s: %#v %#v", tc.file, tools, task)
		}
	}
}

func TestDiagnosticUnicodeRange(t *testing.T) {
	r := languageReport{"sample.py", "x='한😀'; bad", []languageDiagnostic{{lspRange{lspPos{0, 9}, lspPos{0, 12}}, 2, "unknown"}}}
	items := r.problems()
	if len(items) != 1 || items[0].kind != "warning" || items[0].first != 14 || items[0].last != 16 {
		t.Fatal(items)
	}
	wire, _ := json.Marshal(r.items[0].Range)
	if !strings.Contains(string(wire), `"start":{"line":0,"character":9}`) {
		t.Fatal(string(wire))
	}
}

func TestWebCompletionFields(t *testing.T) {
	for _, tc := range []struct {
		source, text, selection string
		cursor                  int
	}{
		{"color: ${1:red};$0", "color: red;", "red", 7},
		{"<button>$0</button>", "<button></button>", "", 8},
		{"body", "body", "", 4},
	} {
		s, a, b, ok := webSnippet(tc.source)
		if !ok || s != tc.text || a != tc.cursor || s[a:b] != tc.selection {
			t.Fatal(tc.source, s, a, b)
		}
	}
}

func TestPreviewProjectPath(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "index.html"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ path, entry string }{{"css/style.css", "index.html"}, {"pages/a #.html", "pages/a #.html"}, {"pages/script.js", "index.html"}} {
		file := filepath.Join(root, filepath.FromSlash(tc.path))
		if entry := filepath.ToSlash(previewEntry(root, file)); entry != tc.entry {
			t.Fatalf("%s: %s", tc.path, entry)
		}
	}
}

func TestPythonDefinitionAndDiagnostics(t *testing.T) {
	if os.Getenv("GALAXY_LANGUAGE_TESTS") != "1" {
		t.Skip("requires installed language servers")
	}
	if _, err := exec.LookPath("pyright-langserver"); err != nil {
		t.Skip("pyright unavailable")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.py")
	src := "def galaxy_total():\n    return 7\nprint(galaxy_total())\n"
	if err := os.WriteFile(path, []byte(src), 0644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	reports := make(chan languageReport, 8)
	client := languageClient{onReport: func(r languageReport) {
		select {
		case reports <- r:
		default:
		}
	}}
	defer client.close()
	raw, err := client.query(ctx, ctx, nativeProject{root: dir}, map[string]string{path: src}, path, lspPos{2, 9}, "definition")
	var defs []location
	if err != nil || json.Unmarshal(raw, &defs) != nil || len(defs) != 1 || defs[0].Range.Start != (lspPos{0, 4}) {
		t.Fatalf("definition: %s, %v", raw, err)
	}
	broken := src + "other = )\n"
	_, err = client.query(ctx, ctx, nativeProject{root: dir}, map[string]string{path: broken}, path, lspPos{}, "diagnostic")
	if err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case r := <-reports:
			if r.text != broken {
				continue
			}
			for _, p := range r.problems() {
				if p.line == 4 && p.kind == "error" {
					return
				}
			}
		case <-ctx.Done():
			t.Fatal("no diagnostic for unsaved line 4")
		}
	}
}

func TestInstalledLanguageServers(t *testing.T) {
	if os.Getenv("GALAXY_LANGUAGE_TESTS") != "1" {
		t.Skip("set GALAXY_LANGUAGE_TESTS=1 to launch installed language servers")
	}
	for _, tc := range []struct{ file, initial, prefix, expected string }{
		{"Demo.java", "class Demo { int galaxyTotal; int f(){return galaxyTo;} }", "galaxyTo;", "galaxyTotal"},
		{"demo.js", "const galaxyTotal = 3;\nconsole.log(galaxyTo);", "galaxyTo)", "galaxyTotal"},
		{"index.html", "<!doctype html>\n<html><body><but</body></html>", "but<", "button"},
		{"theme.css", "body { backgro }", "backgro ", "background"},
		{"demo.go", "package demo\nvar galaxyTotal = 3\nvar x = galaxyTo\n", "galaxyTo\n", "galaxyTotal"},
		{"demo.py", "galaxyTotal = 3\nprint(galaxyTo)", "galaxyTo)", "galaxyTotal"},
		{"src/main.rs", "fn galaxy_total() {}\nfn main(){ galaxy_to }", "galaxy_to ", "galaxy_total"},
	} {
		t.Run(toolsFor(tc.file).id, func(t *testing.T) {
			server := toolsFor(tc.file).server
			if _, err := exec.LookPath(server); err != nil {
				t.Skip(server + " unavailable")
			}
			dir := t.TempDir()
			put := func(path, text string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, path), []byte(text), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if server == "gopls" {
				put("go.mod", "module example.org/demo\n\ngo 1.20\n")
			}
			if server == "rust-analyzer" {
				if err := os.Mkdir(filepath.Join(dir, "src"), 0755); err != nil {
					t.Fatal(err)
				}
				put("Cargo.toml", "[package]\nname='demo'\nversion='0.1.0'\nedition='2021'\n")
			}
			put(tc.file, tc.initial)
			path := filepath.Join(dir, filepath.FromSlash(tc.file))
			at := strings.LastIndex(tc.initial, tc.prefix) + len(tc.prefix) - 1
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			var client languageClient
			defer client.close()
			for {
				raw, err := client.query(ctx, ctx, nativeProject{root: dir}, map[string]string{path: tc.initial}, path, lspPosition(tc.initial, at), "completion")
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(raw), tc.expected) {
					return
				}
				select {
				case <-time.After(350 * time.Millisecond):
				case <-ctx.Done():
					t.Fatalf("missing %s: %.300s", tc.expected, raw)
				}
			}
		})
	}
}
