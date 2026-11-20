package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type languageTools struct {
	id, server string
	args       []string
}

// The web servers return numeric tab stops. Select the first field after insertion.
func webSnippet(s string) (text string, first, last int, ok bool) {
	var out strings.Builder
	best := -1
	values := map[int]string{}
	for at := 0; at < len(s); {
		ch := s[at]
		at++
		if ch == '\\' && at < len(s) && strings.ContainsRune("\\$}", rune(s[at])) {
			out.WriteByte(s[at])
			at++
			continue
		}
		if ch != '$' {
			out.WriteByte(ch)
			continue
		}
		braces := at < len(s) && s[at] == '{'
		if braces {
			at++
		}
		start := at
		for at < len(s) && s[at] >= '0' && s[at] <= '9' {
			at++
		}
		n, err := strconv.Atoi(s[start:at])
		if err != nil {
			return "", 0, 0, false
		}
		value := values[n]
		if braces {
			if at < len(s) && s[at] == ':' {
				at++
				end := strings.IndexByte(s[at:], '}')
				if end < 0 || strings.ContainsAny(s[at:at+end], "$\\") {
					return "", 0, 0, false
				}
				value = s[at : at+end]
				values[n] = value
				at += end
			}
			if at == len(s) || s[at] != '}' {
				return "", 0, 0, false
			}
			at++
		}
		pos := out.Len()
		out.WriteString(value)
		if best < 0 || n > 0 && (best == 0 || n < best) {
			first, last = pos, out.Len()
			best = n
		}
	}
	if best < 0 {
		first, last = out.Len(), out.Len()
	}
	return out.String(), first, last, true
}

func toolsFor(path string) languageTools {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".c", ".cpp", ".cc", ".cxx", ".h", ".hh", ".hpp", ".hxx", ".ipp":
		id := "cpp"
		if nativeLanguage(path) == "c" {
			id = "c"
		}
		return languageTools{id, "clangd", []string{"-j=2", "--background-index", "--header-insertion=never", "--limit-results=40", "--log=error"}}
	case ".java":
		return languageTools{"java", "jdtls", nil}
	case ".js", ".mjs", ".cjs":
		return languageTools{"javascript", "typescript-language-server", []string{"--stdio"}}
	case ".html", ".htm":
		return languageTools{"html", "vscode-html-language-server", []string{"--stdio"}}
	case ".css":
		return languageTools{"css", "vscode-css-language-server", []string{"--stdio"}}
	case ".go":
		return languageTools{"go", "gopls", nil}
	case ".py", ".pyw":
		return languageTools{"python", "pyright-langserver", []string{"--stdio"}}
	case ".rs":
		return languageTools{"rust", "rust-analyzer", nil}
	}
	return languageTools{}
}

func fileTask(path, root string) (task, bool) {
	var args []string
	name := "현재 파일 실행"
	switch toolsFor(path).id {
	case "java":
		args = []string{"java", "{file}"}
	case "javascript":
		args = []string{"node", "{file}"}
	case "python":
		args = []string{"python3", "{file}"}
	case "go":
		args = []string{"go", "run", "{file}"}
	case "rust":
		args = []string{"sh", "-c", `set -eu
d=$(mktemp -d)
trap 'rm -f -- "$d/program"; rmdir -- "$d"' EXIT
rustc --crate-name galaxy_main --edition=2021 "$1" -o "$d/program"
"$d/program"`, "galaxy-rust", "{file}"}
	case "html", "css":
		name = "브라우저 미리보기 서버"
		rel, err := filepath.Rel(root, path)
		if err != nil || !filepath.IsLocal(rel) {
			root = filepath.Dir(path)
			rel = filepath.Base(path)
		}
		if toolsFor(path).id == "css" {
			dir := filepath.Dir(rel)
			rel = ""
			for {
				index := filepath.Join(dir, "index.html")
				if info, err := os.Stat(filepath.Join(root, index)); err == nil && info.Mode().IsRegular() {
					rel = index
					break
				}
				if dir == "." {
					break
				}
				dir = filepath.Dir(dir)
			}
		}
		entry := (&url.URL{Path: filepath.ToSlash(rel)}).EscapedPath()
		args = []string{"python3", "-c", `import http.server, functools, sys
handler = functools.partial(http.server.SimpleHTTPRequestHandler, directory=sys.argv[1])
with http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler) as server:
    print("Galaxy Terminal 브라우저: http://127.0.0.1:%d/%s  (Ctrl+C 종료)" % (server.server_port, sys.argv[2]), flush=True)
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass`, root, entry}
	default:
		if strings.ToLower(filepath.Ext(path)) != ".sh" {
			return task{}, false
		}
		args = []string{"bash", "{file}"}
	}
	return task{Name: name, Args: args}, true
}
