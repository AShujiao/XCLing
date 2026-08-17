package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"XCLing/internal/model"
	"XCLing/internal/store"
)

type ShutdownConfig struct {
	Enabled   bool   `json:"enabled"`
	Hour      int    `json:"hour"`
	CreatedAt string `json:"createdAt"`
}

type ShutdownService struct {
	mu       sync.Mutex
	now      func() time.Time
	stopChan chan struct{}
	running  bool
}

func NewShutdownService() *ShutdownService {
	return &ShutdownService{
		now:      time.Now,
		stopChan: make(chan struct{}),
	}
}

func (s *ShutdownService) configPath() (string, error) {
	if runtime.GOOS != "windows" {
		return "", errors.New("定时关机仅支持 Windows")
	}
	dir, err := store.DataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shutdown-config.json"), nil
}

func (s *ShutdownService) GetConfig() (ShutdownConfig, error) {
	path, err := s.configPath()
	if err != nil {
		return ShutdownConfig{Enabled: false}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ShutdownConfig{Enabled: false}, nil
	}
	if err != nil {
		return ShutdownConfig{Enabled: false}, err
	}
	var cfg ShutdownConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ShutdownConfig{Enabled: false}, nil
	}
	if cfg.Hour < 0 || cfg.Hour > 23 {
		cfg.Hour = 0
	}
	return cfg, nil
}

func (s *ShutdownService) SetConfig(enabled bool, hour int) error {
	if runtime.GOOS != "windows" {
		return errors.New("定时关机仅支持 Windows")
	}
	if hour < 0 || hour > 23 {
		return errors.New("小时必须在 0-23 之间")
	}
	path, err := s.configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	cfg := ShutdownConfig{
		Enabled:   enabled,
		Hour:      hour,
		CreatedAt: s.now().Local().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.stopTimerLocked()
	if enabled {
		s.startTimerLocked(hour)
	}
	return nil
}

func (s *ShutdownService) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopTimerLocked()
}

func (s *ShutdownService) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	if cfg.Enabled {
		s.startTimerLocked(cfg.Hour)
	}
	return nil
}

func (s *ShutdownService) stopTimerLocked() {
	if s.running {
		close(s.stopChan)
		s.stopChan = make(chan struct{})
		s.running = false
	}
}

func (s *ShutdownService) startTimerLocked(hour int) {
	s.stopTimerLocked()
	s.running = true
	stopChan := s.stopChan

	go func() {
		for {
			now := s.now()
			next := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
			if !next.After(now) {
				next = next.Add(24 * time.Hour)
			}
			duration := next.Sub(now)

			timer := time.NewTimer(duration)
			select {
			case <-timer.C:
				s.executeShutdown(hour)
				timer.Reset(24 * time.Hour)
				continue
			case <-stopChan:
				timer.Stop()
				return
			}
		}
	}()
}

func (s *ShutdownService) executeShutdown(hour int) {
	message := fmt.Sprintf("定时关机：已到设定时间 %02d:00，系统将在 60 秒后关机", hour)
	s.recordShutdownEvent(message)
	cmd := exec.Command("shutdown", "/s", "/t", "60", "/c", message)
	_ = cmd.Run()
}

func (s *ShutdownService) recordShutdownEvent(message string) {
	if runtime.GOOS != "windows" {
		return
	}
	events, err := store.NewProtectionEventStore()
	if err != nil {
		return
	}
	createdAt := s.now().Local().Format(time.RFC3339Nano)
	_ = events.Append(model.ProtectionEvent{
		ID:        createdAt,
		Action:    "shutdown",
		Success:   true,
		Message:   message,
		CreatedAt: createdAt,
	})
}

func (s *ShutdownService) CancelShutdown() error {
	if runtime.GOOS != "windows" {
		return errors.New("仅支持 Windows")
	}
	cmd := exec.Command("shutdown", "/a")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("取消关机失败: %s", string(output))
	}
	s.recordShutdownEvent("已取消定时关机")
	return nil
}
