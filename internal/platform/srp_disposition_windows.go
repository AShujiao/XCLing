//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"XCLing/internal/model"

	"golang.org/x/sys/windows/registry"
)

func InspectSRPRoot() (SRPRootSnapshot, error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath, registry.READ|registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return SRPRootSnapshot{}, nil
		}
		return SRPRootSnapshot{}, err
	}
	defer key.Close()

	snapshot := SRPRootSnapshot{Exists: true, Values: []SRPRootValue{}, Children: []string{}, LevelChildren: []string{}, BlockChildren: []string{}}
	valueNames, err := key.ReadValueNames(-1)
	if err != nil {
		return SRPRootSnapshot{}, fmt.Errorf("read SRP root values: %w", err)
	}
	for _, name := range valueNames {
		_, valueType, err := key.GetValue(name, nil)
		if err != nil {
			return SRPRootSnapshot{}, fmt.Errorf("read SRP root value %q type: %w", name, err)
		}
		value := SRPRootValue{Name: name, Kind: fmt.Sprintf("type_%d", valueType)}
		if valueType == registry.DWORD {
			integer, actualType, err := key.GetIntegerValue(name)
			if err != nil {
				return SRPRootSnapshot{}, fmt.Errorf("read SRP DWORD %q: %w", name, err)
			}
			if actualType != registry.DWORD {
				return SRPRootSnapshot{}, fmt.Errorf("read SRP DWORD %q: unexpected type %d", name, actualType)
			}
			value.Kind, value.Integer = SRPValueDWORD, integer
		} else if valueType == registry.MULTI_SZ {
			values, actualType, stringsErr := key.GetStringsValue(name)
			if stringsErr != nil {
				return SRPRootSnapshot{}, fmt.Errorf("read SRP MULTI_SZ %q: %w", name, stringsErr)
			}
			if actualType != registry.MULTI_SZ {
				return SRPRootSnapshot{}, fmt.Errorf("read SRP MULTI_SZ %q: unexpected type %d", name, actualType)
			}
			value.Kind, value.Strings = SRPValueMULTISZ, values
		}
		snapshot.Values = append(snapshot.Values, value)
	}
	children, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return SRPRootSnapshot{}, fmt.Errorf("read SRP root children: %w", err)
	}
	snapshot.Children = append(snapshot.Children, children...)
	for _, child := range children {
		isUnrestricted := strings.EqualFold(child, "262144")
		isDisallowed := strings.EqualFold(child, "0")
		if !isUnrestricted && !isDisallowed {
			continue
		}
		level, err := registry.OpenKey(registry.LOCAL_MACHINE, model.SRPRegistryPath+`\`+child, registry.READ|registry.ENUMERATE_SUB_KEYS)
		if err != nil {
			return SRPRootSnapshot{}, fmt.Errorf("open SRP level %s: %w", child, err)
		}
		levelChildren, readErr := level.ReadSubKeyNames(-1)
		_ = level.Close()
		if readErr != nil {
			return SRPRootSnapshot{}, fmt.Errorf("read SRP level %s children: %w", child, readErr)
		}
		if isUnrestricted {
			snapshot.LevelChildren = append(snapshot.LevelChildren, levelChildren...)
		} else {
			snapshot.BlockChildren = append(snapshot.BlockChildren, levelChildren...)
		}
	}
	return snapshot, nil
}
