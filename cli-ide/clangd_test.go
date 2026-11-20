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
)

func TestLSPPositions(t *testing.T) {
	text := "start\n한😀ab"
	p := lspPosition(text, 13)
	if p.Line != 1 || p.Character != 3 {
		t.Fatal(p)
	}
	if off, ok := bytePosition(text, p); !ok || off != 13 {
		t.Fatal(off, ok)
	}
	if _, ok := bytePosition(text, lspPos{1, 2}); ok {
		t.Fatal("accepted half of a surrogate pair")
	}
	f := filepath.Join(t.TempDir(), "한 글#%.c")
	if got, err := uriPath(fileURI(f)); err != nil || got != f {
		t.Fatal(got, err)
	}
}

func TestClangdTwoFileProject(t *testing.T) {
	if _, err := exec.LookPath("clangd"); err != nil {
		t.Skip("requires clangd")
	}
	for _, ext := range []string{".c", ".cpp"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			main := filepath.Join(dir, "main"+ext)
			helper := filepath.Join(dir, "helper"+ext)
			saved := "int galaxy_add(int,int);\nint main(void){return galaxy_add(2,3);}\n"
			for path, text := range map[string]string{main: saved, helper: "// helper\nint galaxy_add(int a,int b){return a+b;}\n"} {
				if err := os.WriteFile(path, []byte(text), 0644); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			var client languageClient
			defer client.close()
			project := nativeProject{root: dir}
			edited := strings.Replace(saved, "return galaxy_add", "return galaxy_a", 1)
			raw, err := client.query(ctx, ctx, project, map[string]string{main: edited}, main, lspPosition(edited, strings.LastIndex(edited, "galaxy_a")+len("galaxy_a")), "completion")
			if err != nil || !strings.Contains(string(raw), "galaxy_add") {
				t.Fatalf("completion: %s %v", raw, err)
			}
			for {
				raw, err = client.query(ctx, ctx, project, map[string]string{main: saved}, main, lspPosition(saved, strings.LastIndex(saved, "galaxy_add")+3), "definition")
				var defs []location
				if err != nil {
					t.Fatal(err)
				}
				if err = json.Unmarshal(raw, &defs); err != nil {
					t.Fatal(err)
				}
				if len(defs) > 0 && defs[0].URI == fileURI(helper) && defs[0].Range.Start.Line == 1 {
					break
				}
				select {
				case <-ctx.Done():
					t.Fatalf("definition: %s", raw)
				case <-time.After(200 * time.Millisecond):
				}
			}
		})
	}
}
