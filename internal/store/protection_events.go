package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"XCLing/internal/model"
)

const maxProtectionEvents = 200

var protectionEventsMu sync.Mutex

// ProtectionEventStore persists a bounded local lifecycle history.
type ProtectionEventStore struct {
	path   string
	secure bool
}

// NewProtectionEventStore creates the ProgramData-backed event store.
func NewProtectionEventStore() (*ProtectionEventStore, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("操作记录仅支持 Windows")
	}
	base := os.Getenv("ProgramData")
	if base == "" {
		return nil, errors.New("ProgramData 未设置")
	}
	return &ProtectionEventStore{path: filepath.Join(base, appDirName, "activity", "events.json"), secure: true}, nil
}

// NewProtectionEventStoreAt creates a store for tests.
func NewProtectionEventStoreAt(path string) *ProtectionEventStore {
	return &ProtectionEventStore{path: path}
}

// List returns newest events first.
func (s *ProtectionEventStore) List() ([]model.ProtectionEvent, error) {
	protectionEventsMu.Lock()
	defer protectionEventsMu.Unlock()
	events, err := s.load()
	if errors.Is(err, os.ErrNotExist) {
		return []model.ProtectionEvent{}, nil
	}
	return events, err
}

// Append adds an event and keeps only the newest bounded history.
func (s *ProtectionEventStore) Append(event model.ProtectionEvent) error {
	protectionEventsMu.Lock()
	defer protectionEventsMu.Unlock()
	events, err := s.load()
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	events = append([]model.ProtectionEvent{event}, events...)
	if len(events) > maxProtectionEvents {
		events = events[:maxProtectionEvents]
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	if s.secure {
		if err := secureRecoveryPath(filepath.Dir(s.path)); err != nil {
			return fmt.Errorf("secure activity directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if s.secure {
		if err := secureRecoveryPath(tmp); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("secure activity file: %w", err)
		}
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func (s *ProtectionEventStore) load() ([]model.ProtectionEvent, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return []model.ProtectionEvent{}, err
	}
	var events []model.ProtectionEvent
	if err := json.Unmarshal(data, &events); err != nil {
		return nil, err
	}
	return events, nil
}
