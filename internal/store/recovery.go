package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"XCLing/internal/model"
)

type RecoveryStore struct {
	dir    string
	secure bool
}

func NewRecoveryStore() (*RecoveryStore, error) {
	if runtime.GOOS != "windows" {
		return nil, errors.New("恢复存储仅支持 Windows")
	}
	base := os.Getenv("ProgramData")
	if base == "" {
		return nil, errors.New("ProgramData 未设置")
	}
	return &RecoveryStore{dir: filepath.Join(base, appDirName, "recovery"), secure: true}, nil
}

func NewRecoveryStoreAt(dir string) *RecoveryStore { return &RecoveryStore{dir: dir} }
func (s *RecoveryStore) Path() string              { return filepath.Join(s.dir, "active.json") }

func (s *RecoveryStore) Load() (model.RecoveryRecord, error) {
	data, err := os.ReadFile(s.Path())
	if err != nil {
		return model.RecoveryRecord{}, err
	}
	var record model.RecoveryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return model.RecoveryRecord{}, err
	}
	if record.SchemaVersion != "1" && record.SchemaVersion != "2" {
		return model.RecoveryRecord{}, errors.New("unsupported recovery schema")
	}
	if record.SchemaVersion == "2" {
		switch record.BeforeState {
		case model.BeforeStateAbsent:
			if record.BeforeSnapshot.Exists {
				return model.RecoveryRecord{}, errors.New("invalid absent recovery snapshot")
			}
		case model.BeforeStateInert, model.BeforeStateManaged:
			if !record.BeforeSnapshot.Exists {
				return model.RecoveryRecord{}, errors.New("invalid existing recovery snapshot")
			}
		default:
			return model.RecoveryRecord{}, errors.New("unsupported recovery beforeState")
		}
		return record, nil
	}
	switch record.BeforeState {
	case model.BeforeStateAbsent:
		if record.BeforeDefaultLevel != 0 {
			return model.RecoveryRecord{}, errors.New("invalid absent recovery snapshot")
		}
	case model.BeforeStateInert:
		if record.BeforeDefaultLevel != model.SrpLevelUnrestrictedRaw {
			return model.RecoveryRecord{}, errors.New("invalid inert recovery snapshot")
		}
	default:
		return model.RecoveryRecord{}, errors.New("unsupported recovery beforeState")
	}
	return record, nil
}

func (s *RecoveryStore) Save(record model.RecoveryRecord) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return err
	}
	if s.secure {
		if err := secureRecoveryPath(s.dir); err != nil {
			return fmt.Errorf("secure recovery directory: %w", err)
		}
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.Path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if s.secure {
		if err := secureRecoveryPath(tmp); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("secure recovery file: %w", err)
		}
	}
	if err := os.Rename(tmp, s.Path()); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
