// Package store owns XCLing's user-data directory and secure JSON file protocol.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const appDirName = "XCLing"

var (
	dataDirOnce sync.Once
	dataDir     string
	dataDirErr  error
)

func DataDir() (string, error) {
	dataDirOnce.Do(func() {
		dataDir, dataDirErr = resolveDataDir()
		if dataDirErr == nil {
			dataDirErr = os.MkdirAll(dataDir, 0o700)
		}
	})
	return dataDir, dataDirErr
}

func resolveDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		base := os.Getenv("AppData")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			base = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(base, appDirName), nil
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support", appDirName), nil
	default:
		if base := os.Getenv("XDG_CONFIG_HOME"); base != "" {
			return filepath.Join(base, appDirName), nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".config", appDirName), nil
	}
}

// JSONFiles hides path validation, permissions and atomic replacement for one data kind.
type JSONFiles struct {
	dir    string
	suffix string
	mu     sync.Mutex
}

func NewJSONFiles(dir, suffix string) *JSONFiles {
	return &JSONFiles{dir: dir, suffix: suffix}
}

func (s *JSONFiles) Dir() string { return s.dir }

func ValidID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, char := range id {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_') {
			return false
		}
	}
	return true
}

func (s *JSONFiles) Path(id string) (string, error) {
	if !ValidID(id) {
		return "", fmt.Errorf("invalid data id: %q", id)
	}
	return filepath.Join(s.dir, id+s.suffix), nil
}

func (s *JSONFiles) Save(id string, value any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.Path(id)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("ensure data directory: %w", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return fmt.Errorf("write temporary data: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace data: %w", err)
	}
	return nil
}

func (s *JSONFiles) Load(id string, target any) error {
	if target == nil {
		return errors.New("load target must not be nil")
	}
	path, err := s.Path(id)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func (s *JSONFiles) IDs() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), s.suffix) {
			ids = append(ids, strings.TrimSuffix(entry.Name(), s.suffix))
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *JSONFiles) Exists(id string) bool {
	path, err := s.Path(id)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

func (s *JSONFiles) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, err := s.Path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
