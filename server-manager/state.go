package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

type profile struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Dir     string `json:"working_directory"`
	URL     string `json:"browser_url,omitempty"`
}

var validID = regexp.MustCompile("^[A-Z2-7]{26}$")

func openProject(project string) (string, string, error) {
	root, err := filepath.Abs(project)
	if err != nil {
		return "", "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", "", err
	}
	if err := checkDirectory(root); err != nil {
		return "", "", err
	}
	state := os.Getenv("XDG_STATE_HOME")
	if !filepath.IsAbs(state) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		state = filepath.Join(home, ".local", "state")
	}
	key := sha256.Sum256([]byte(root))
	dir := filepath.Join(state, "galaxy-servers", fmt.Sprintf("%x", key[:16]))
	return root, dir, os.MkdirAll(dir, 0700)
}

func checkDirectory(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("폴더가 아닙니다: %s", path)
	}
	return nil
}

func readProfiles(dir string) ([]profile, error) {
	data, err := os.ReadFile(filepath.Join(dir, "profiles.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var profiles []profile
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, fmt.Errorf("서버 설정 읽기: %w", err)
	}
	seen := make(map[string]bool)
	for _, p := range profiles {
		if !validID.MatchString(p.ID) || seen[p.ID] {
			return nil, errors.New("서버 설정에 잘못된 ID가 있습니다")
		}
		seen[p.ID] = true
	}
	return profiles, nil
}

func saveProfiles(dir string, profiles []profile) error {
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	return writeState(filepath.Join(dir, "profiles.json"), append(data, '\n'))
}

func writeState(path string, data []byte) error {
	stage, err := os.CreateTemp(filepath.Dir(path), ".stage-*")
	if err != nil {
		return err
	}
	defer os.Remove(stage.Name())
	_, err = stage.Write(data)
	if err == nil {
		err = stage.Sync()
	}
	if err = errors.Join(err, stage.Close()); err != nil {
		return err
	}
	return os.Rename(stage.Name(), path)
}

// The locked file itself stays in place so every opener uses the same inode.
func takeLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func settings(root string, previous profile, name, command, dir, browser string) (profile, error) {
	name, command, dir, browser = strings.TrimSpace(name), strings.TrimSpace(command), strings.TrimSpace(dir), strings.TrimSpace(browser)
	if name == "" || command == "" {
		return profile{}, errors.New("이름과 실행 명령을 입력하세요")
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	if err := checkDirectory(dir); err != nil {
		return profile{}, err
	}
	if browser != "" && !validBrowserURL(browser) {
		return profile{}, errors.New("http://127.0.0.1:3000 같은 로컬 주소를 입력하세요")
	}
	id := previous.ID
	if id == "" {
		id = rand.Text()
	}
	return profile{ID: id, Name: name, Command: command, Dir: dir, URL: browser}, nil
}

func validBrowserURL(value string) bool {
	// tview URL tags can't represent brackets, spaces, or non-ASCII bytes.
	for _, ch := range value {
		if ch < 33 || ch >= 127 || ch == '[' || ch == ']' {
			return false
		}
	}
	u, err := url.Parse(value)
	if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	switch strings.ToLower(u.Hostname()) {
	case "localhost", "127.0.0.1":
		return true
	}
	return false
}
