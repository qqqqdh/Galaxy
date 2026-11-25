//go:build ignore

// Prepare the terminal library patch used by release builds.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func main() {
	if err := prepare(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func prepare() error {
	patch, err := filepath.Abs("patches/terminal-scrolling.patch")
	if err != nil {
		return err
	}
	stage, err := os.MkdirTemp("", "galaxy-ide-overlay-")
	if err != nil {
		return err
	}
	replace := make(map[string]string)
	for _, entry := range []struct{ module, path string }{
		{"github.com/charmbracelet/x/vt", "vt/scrollback.go"},
		{"github.com/charmbracelet/ultraviolet", "ultraviolet/buffer.go"},
	} {
		module := exec.Command("go", "list", "-m", "-f", "{{.Path}}@{{.Version}}", entry.module)
		module.Stderr = os.Stderr
		version, err := module.Output()
		if err != nil {
			return err
		}
		fetch := exec.Command("go", "mod", "download", "-json", strings.TrimSpace(string(version)))
		fetch.Stderr = os.Stderr
		output, err := fetch.Output()
		if err != nil {
			return err
		}
		var downloaded struct{ Dir string }
		if err := json.Unmarshal(output, &downloaded); err != nil {
			return err
		}
		source := filepath.Join(downloaded.Dir, filepath.Base(entry.path))
		target := filepath.Join(stage, entry.path)
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return err
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return err
		}
		replace[source] = target
	}
	apply := exec.Command("git", "apply", "--no-index", patch)
	apply.Dir = stage
	apply.Stderr = os.Stderr
	if err := apply.Run(); err != nil {
		return err
	}
	data, err := json.Marshal(struct{ Replace map[string]string }{replace})
	if err != nil {
		return err
	}
	overlay := filepath.Join(stage, "overlay.json")
	if err := os.WriteFile(overlay, data, 0600); err != nil {
		return err
	}
	fmt.Println(overlay)
	return nil
}
