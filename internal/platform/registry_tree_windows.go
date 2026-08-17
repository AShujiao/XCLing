//go:build windows

package platform

import (
	"errors"
	"fmt"
	"os"

	"XCLing/internal/model"

	"golang.org/x/sys/windows/registry"
)

func readRegistryTree(root registry.Key, path string) (model.RegistryTreeSnapshot, error) {
	key, err := registry.OpenKey(root, path, registry.READ|registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) || errors.Is(err, os.ErrNotExist) {
			return model.RegistryTreeSnapshot{}, nil
		}
		return model.RegistryTreeSnapshot{}, fmt.Errorf("open registry tree %q: %w", path, err)
	}
	defer key.Close()

	snapshot, err := readRegistryKey(key, path)
	if err != nil {
		return model.RegistryTreeSnapshot{}, err
	}
	return model.RegistryTreeSnapshot{Exists: true, Root: snapshot}, nil
}

func readRegistryKey(key registry.Key, path string) (model.RegistryKeySnapshot, error) {
	result := model.RegistryKeySnapshot{Values: []model.RegistryValueSnapshot{}, Children: []model.RegistryNamedKeySnapshot{}}
	valueNames, err := key.ReadValueNames(-1)
	if err != nil {
		return model.RegistryKeySnapshot{}, fmt.Errorf("read values from %q: %w", path, err)
	}
	for _, name := range valueNames {
		size, valueType, readErr := key.GetValue(name, nil)
		if readErr != nil && !errors.Is(readErr, registry.ErrShortBuffer) {
			return model.RegistryKeySnapshot{}, fmt.Errorf("size registry value %q in %q: %w", name, path, readErr)
		}
		data := make([]byte, size)
		if size > 0 {
			actual, actualType, valueErr := key.GetValue(name, data)
			if valueErr != nil {
				return model.RegistryKeySnapshot{}, fmt.Errorf("read registry value %q in %q: %w", name, path, valueErr)
			}
			data, valueType = data[:actual], actualType
		}
		result.Values = append(result.Values, model.RegistryValueSnapshot{Name: name, Type: valueType, Data: data})
	}
	childNames, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return model.RegistryKeySnapshot{}, fmt.Errorf("read children from %q: %w", path, err)
	}
	for _, name := range childNames {
		child, openErr := registry.OpenKey(key, name, registry.READ|registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE)
		if openErr != nil {
			return model.RegistryKeySnapshot{}, fmt.Errorf("open child %q in %q: %w", name, path, openErr)
		}
		defer child.Close()

		childSnapshot, childErr := readRegistryKey(child, path+`\`+name)
		if childErr != nil {
			return model.RegistryKeySnapshot{}, childErr
		}
		result.Children = append(result.Children, model.RegistryNamedKeySnapshot{Name: name, Key: childSnapshot})
	}
	return result, nil
}

// SnapshotSRPRegistry reads the complete local SRP registry tree without modifying it.
func SnapshotSRPRegistry() (model.RegistryTreeSnapshot, error) {
	return readRegistryTree(registry.LOCAL_MACHINE, model.SRPRegistryPath)
}
