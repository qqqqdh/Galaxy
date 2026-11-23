package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type task struct {
	Name        string   `json:"name"`
	Args        []string `json:"args"`
	Group       string   `json:"group,omitempty"`
	buildAndRun bool
	assistant   bool
}

type config struct {
	IndentSize int      `json:"indent_size"`
	UseTabs    bool     `json:"use_tabs"`
	Tasks      []task   `json:"tasks"`
	Ignore     []string `json:"ignore"`
}

func loadConfig(root string) (config, error) {
	c := config{IndentSize: 4, Ignore: []string{".git", "node_modules", ".venv", "venv", "__pycache__", ".build-cache"}}
	dir, err := os.UserConfigDir()
	if err != nil {
		return c, err
	}
	for _, path := range []string{filepath.Join(dir, "galaxy-ide", "config.json"), filepath.Join(root, ".galaxy-ide.json")} {
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return c, err
		}
		if err := json.Unmarshal(data, &c); err != nil {
			return c, fmt.Errorf("%s: %w", path, err)
		}
	}
	if c.IndentSize < 1 || c.IndentSize > 8 {
		return c, fmt.Errorf("indent_size는 1부터 8까지 가능합니다")
	}
	groups := make(map[string]bool)
	for _, t := range c.Tasks {
		if strings.TrimSpace(t.Name) == "" || len(t.Args) == 0 || strings.TrimSpace(t.Args[0]) == "" {
			return c, fmt.Errorf("각 task에는 name과 비어 있지 않은 args가 필요합니다")
		}
		if t.Group != "" {
			if t.Group != "build" && t.Group != "test" {
				return c, fmt.Errorf("%s: group에는 build 또는 test를 지정하세요", t.Name)
			}
			if groups[t.Group] {
				return c, fmt.Errorf("%s 그룹에는 한 작업만 지정할 수 있습니다", t.Group)
			}
			groups[t.Group] = true
		}
	}
	return c, nil
}

func (c config) groupTask(group string) (task, bool) {
	for _, t := range c.Tasks {
		if t.Group == group {
			return t, true
		}
	}
	return task{}, false
}

func expandArgs(args []string, file, root string) ([]string, error) {
	result := make([]string, len(args))
	for i, arg := range args {
		if strings.Contains(arg, "{file}") && file == "" {
			return nil, fmt.Errorf("이 작업을 실행하려면 파일을 먼저 저장하세요")
		}
		result[i] = strings.NewReplacer("{file}", file, "{root}", root).Replace(arg)
	}
	return result, nil
}
