package model

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

// RegistryValueSnapshot preserves one registry value exactly as its type and raw bytes.
type RegistryValueSnapshot struct {
	Name string `json:"name"`
	Type uint32 `json:"type"`
	Data []byte `json:"data"`
}

// RegistryNamedKeySnapshot preserves a named child key and its contents.
type RegistryNamedKeySnapshot struct {
	Name string              `json:"name"`
	Key  RegistryKeySnapshot `json:"key"`
}

// RegistryKeySnapshot is a recursive, order-independent registry key snapshot.
type RegistryKeySnapshot struct {
	Values   []RegistryValueSnapshot    `json:"values"`
	Children []RegistryNamedKeySnapshot `json:"children"`
}

// RegistryTreeSnapshot records whether the SRP root existed and, if so, its full tree.
type RegistryTreeSnapshot struct {
	Exists bool                `json:"exists"`
	Root   RegistryKeySnapshot `json:"root"`
}

// Equal reports whether two snapshots describe the same registry tree.
func (s RegistryTreeSnapshot) Equal(other RegistryTreeSnapshot) bool {
	left, _ := json.Marshal(s.canonical())
	right, _ := json.Marshal(other.canonical())
	return bytes.Equal(left, right)
}

func (s RegistryTreeSnapshot) canonical() RegistryTreeSnapshot {
	return RegistryTreeSnapshot{Exists: s.Exists, Root: canonicalRegistryKey(s.Root)}
}

func canonicalRegistryKey(key RegistryKeySnapshot) RegistryKeySnapshot {
	result := RegistryKeySnapshot{
		Values:   append([]RegistryValueSnapshot(nil), key.Values...),
		Children: make([]RegistryNamedKeySnapshot, len(key.Children)),
	}
	for i, child := range key.Children {
		result.Children[i] = RegistryNamedKeySnapshot{Name: child.Name, Key: canonicalRegistryKey(child.Key)}
	}
	sort.Slice(result.Values, func(i, j int) bool {
		return strings.ToLower(result.Values[i].Name) < strings.ToLower(result.Values[j].Name)
	})
	sort.Slice(result.Children, func(i, j int) bool {
		return strings.ToLower(result.Children[i].Name) < strings.ToLower(result.Children[j].Name)
	})
	for i := range result.Values {
		result.Values[i].Name = strings.ToLower(result.Values[i].Name)
	}
	for i := range result.Children {
		result.Children[i].Name = strings.ToLower(result.Children[i].Name)
	}
	return result
}
